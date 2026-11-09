package agent_client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/paularlott/jsonrpc"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/methods"
)

const (
	defaultMethodTimeoutSeconds = 30
)

// methodServerProcess owns the long-running stdio method server. The JSON-RPC
// framing, response correlation, subprocess lifecycle and reaping are handled
// by the jsonrpc package (NewProcessTransport + Client); this struct holds the
// client, the per-server concurrency policy and the closed signal used to bail
// out of serial-mode acquire promptly when the process dies.
type methodServerProcess struct {
	reg *methods.Registration

	// client talks JSON-RPC over the subprocess's stdin/stdout. It is nil when
	// the server has no backing process (e.g. a unit-test harness that wires a
	// transport directly).
	client    *jsonrpc.Client
	transport *jsonrpc.StreamTransport

	// semaphore caps the number of in-flight requests. nil means unlimited
	// (concurrent mode). A buffered channel of size 1 enforces serial mode.
	semaphore chan struct{}

	// closed is signalled once the subprocess has exited (via the jsonrpc
	// OnExit hook) or the server has been shut down, so serial-mode acquire
	// can fail fast instead of waiting for the per-call timeout.
	closed    chan struct{}
	closeOnce sync.Once
}

func (c *AgentClient) RegisterMethods(reg *methods.Registration) error {
	if reg == nil {
		return fmt.Errorf("registration is required")
	}
	if reg.Server.Command == "" {
		return fmt.Errorf("server command is required")
	}

	// The new server starts alongside the running one, which keeps serving
	// until the knot server accepts the new registration: a refused
	// registration leaves the space's methods as they were.
	server, err := c.newMethodServer(reg)
	if err != nil {
		return err
	}
	if err := c.publishMethods(reg); err != nil {
		server.shutdown()
		return err
	}

	c.methodMu.Lock()
	select {
	case <-server.closed:
		// It exited before taking over, after the knot server had replaced
		// the running server's registration with its own: put that back.
		c.methodMu.Unlock()
		c.restoreMethods()
		return errors.New("method server exited during registration")
	default:
	}
	previous := c.methodServer
	c.methodServer = server
	c.methodMu.Unlock()

	if previous != nil {
		previous.shutdown()
	}

	// Stash so we can re-publish after any knot-server reconnect (the server's
	// registry is in-memory and lost on restart).
	c.lastRegMu.Lock()
	c.lastReg = reg
	c.lastRegMu.Unlock()

	return nil
}

// restoreMethods re-publishes the running server's registration, or drops
// the space's methods when there is none.
func (c *AgentClient) restoreMethods() {
	c.lastRegMu.RLock()
	reg := c.lastReg
	c.lastRegMu.RUnlock()
	if reg == nil {
		c.unregisterMethods()
		return
	}
	if err := c.publishMethods(reg); err != nil {
		log.WithError(err).Warn("restoreMethods: failed to re-publish the running registration")
	}
}

// UnregisterAllMethods removes all methods from the knot server, stops the
// stdio method server process, and clears the stashed registration so
// reconnect doesn't republish dead methods. Called by `knot methods
// unregister` and by the Scriptling server.unregister() (no args).
func (c *AgentClient) UnregisterAllMethods() error {
	// Tell the knot server to drop our methods.
	c.unregisterMethods()

	// Stop the stdio method server process.
	c.stopMethodServer()

	// Clear the stashed registration so republish on reconnect is a no-op.
	c.lastRegMu.Lock()
	c.lastReg = nil
	c.lastRegMu.Unlock()

	return nil
}

// unregisterMethods tells all connected knot servers to remove this space's
// methods from the registry. Called when the stdio method server process
// exits (the methods are no longer callable). Best-effort: if the agent has
// no live connections the methods will be removed when the session drops.
func (c *AgentClient) unregisterMethods() {
	c.serverListMutex.RLock()
	defer c.serverListMutex.RUnlock()

	for _, server := range c.serverList {
		if server.muxSession == nil || server.muxSession.IsClosed() {
			continue
		}
		conn, err := server.muxSession.Open()
		if err != nil {
			continue
		}
		_ = msg.WriteCommand(conn, msg.CmdUnregisterMethods)
		_ = conn.Close()
	}
}

// republishMethods re-publishes the most recent registration to all servers in
// the zone. Called by agentServer.ConnectAndServe after every successful
// (re)connection.
func (c *AgentClient) republishMethods() {
	c.lastRegMu.RLock()
	reg := c.lastReg
	c.lastRegMu.RUnlock()
	if reg == nil {
		return
	}
	if err := c.publishMethods(reg); err != nil {
		log.WithError(err).Warn("republishMethods: failed to re-publish after reconnect")
		return
	}
	log.Info("republished methods after reconnect", "method_count", len(reg.Methods))
}

// newMethodServer starts a method server process for reg. The caller
// installs it as c.methodServer once its registration is accepted.
func (c *AgentClient) newMethodServer(reg *methods.Registration) (*methodServerProcess, error) {
	// stderr is forwarded to the knot server as log lines, one SendLogMessage
	// per newline-delimited chunk (matching the previous pumpStderr behaviour).
	logWriter := &stderrLogWriter{c: c}

	server := &methodServerProcess{
		reg:    reg,
		closed: make(chan struct{}),
	}
	if reg.Server.Mode == methods.ModeSerial {
		server.semaphore = make(chan struct{}, 1)
	}

	// onExit runs once when the subprocess exits — whether it crashes
	// mid-session or is shut down by Close. It mirrors the cmd.Wait goroutine
	// the previous implementation ran: flush any trailing stderr, fail any
	// queued callers, drop the knot-server registrations for the dead process
	// and clear the stashed registration so a reconnect doesn't republish
	// methods whose backing process has gone. A server that is not the
	// running one (replaced, refused or stopped) owns no registration, so its
	// exit leaves the registrations alone.
	onExit := func(waitErr error) {
		logWriter.flush()
		server.markClosed()

		c.methodMu.Lock()
		current := c.methodServer == server
		if current {
			c.methodServer = nil
		}
		c.methodMu.Unlock()

		if current {
			c.unregisterMethods()

			c.lastRegMu.Lock()
			c.lastReg = nil
			c.lastRegMu.Unlock()
		}

		if waitErr != nil {
			log.WithError(waitErr).Warn("method server exited")
		} else {
			log.Info("method server exited")
		}
	}

	transport, err := jsonrpc.NewProcessTransport(
		reg.Server.Command, reg.Server.Args,
		jsonrpc.WithStderr(logWriter),
		jsonrpc.WithOnExit(onExit),
	)
	if err != nil {
		return nil, err
	}

	server.client = jsonrpc.NewClient(transport)
	server.transport = transport

	return server, nil
}

// stopMethodServer stops the running server. Its exit leaves the space's
// registrations on the knot servers; callers drop them where needed.
func (c *AgentClient) stopMethodServer() {
	c.methodMu.Lock()
	server := c.methodServer
	c.methodServer = nil
	c.methodMu.Unlock()

	if server == nil {
		return
	}
	server.shutdown()
}

func (c *AgentClient) publishMethods(reg *methods.Registration) error {
	c.serverListMutex.RLock()
	defer c.serverListMutex.RUnlock()

	if len(c.serverList) == 0 {
		return errors.New("not connected to any knot server")
	}

	published := 0
	var lastErr error
	for _, server := range c.serverList {
		if server.muxSession == nil || server.muxSession.IsClosed() {
			log.Warn("publishMethods: skipping server with no live mux session", "server", server.address)
			continue
		}
		conn, err := server.muxSession.Open()
		if err != nil {
			log.WithError(err).Warn("publishMethods: failed to open mux stream", "server", server.address)
			lastErr = err
			continue
		}
		err = msg.WriteCommand(conn, msg.CmdRegisterMethods)
		if err == nil {
			err = msg.WriteMessage(conn, &msg.RegisterMethodsRequest{Registration: *reg})
		}
		// The server checks and stores the registration before it answers,
		// which a busy database may slow.
		var response msg.RegisterMethodsResponse
		if err == nil {
			err = msg.ReadMessageWithTimeout(conn, &response, msg.RegistrationReplyTimeout)
		}
		_ = conn.Close()
		if err != nil {
			log.WithError(err).Warn("publishMethods: register round-trip failed", "server", server.address)
			lastErr = err
			continue
		}
		if !response.Success {
			log.Warn("publishMethods: server rejected registration", "server", server.address, "error", response.Error)
			lastErr = errors.New(response.Error)
			continue
		}
		log.Debug("publishMethods: registration accepted", "server", server.address, "methods", len(reg.Methods))
		published++
	}

	if published == 0 {
		if lastErr == nil {
			lastErr = errors.New("no live knot server connection available")
		}
		return fmt.Errorf("method registration not published to any server: %w", lastErr)
	}
	return nil
}

// CallMethod forwards a JSON-RPC call to the method server. In concurrent mode
// many calls may be in flight at once; in serial mode calls queue one at a
// time. The caller's original id is preserved on the returned response — the
// jsonrpc client generates its own wire ids, so they never need to be unique.
func (c *AgentClient) CallMethod(req msg.CallMethodRequest) methods.JSONRPCResponse {
	c.methodMu.RLock()
	server := c.methodServer
	c.methodMu.RUnlock()
	if server == nil || server.client == nil {
		return methodError(req.ID, -32000, "no method server available")
	}

	timeout := server.reg.Server.Timeout
	if timeout <= 0 {
		timeout = defaultMethodTimeoutSeconds
	}

	// One deadline gates both the serial-slot acquire and the call itself, so
	// the total time can never exceed the configured per-call timeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	c.activeMethodCalls.Add(1)
	defer c.activeMethodCalls.Add(-1)

	release, acquireErr := server.acquire(ctx)
	if acquireErr != nil {
		return methodError(req.ID, -32000, acquireErr.Error())
	}
	defer release()

	var rawResult json.RawMessage
	err := server.client.Call(ctx, req.Method, req.Params, &rawResult)
	if err == nil {
		c.methodCallsTotal.Add(1)
		c.MarkActivity()
		return methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  toJSONResult(rawResult),
			ID:      req.ID,
		}
	}

	// A JSON-RPC error response from the method server is a completed
	// round-trip — preserve its code/message and count it.
	var rpcErr *jsonrpc.Error
	if errors.As(err, &rpcErr) {
		c.methodCallsTotal.Add(1)
		c.MarkActivity()
		return methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: rpcErr.Code, Message: rpcErr.Message},
			ID:      req.ID,
		}
	}

	// Transport-level failure (process exited, write error, timeout). Surface
	// as a generic server error; this is not a completed round-trip.
	return methodError(req.ID, -32000, err.Error())
}

// SendNotification forwards a JSON-RPC notification (no id) to the method
// server. It returns immediately — no response is expected.
func (c *AgentClient) SendNotification(req msg.CallMethodRequest) {
	c.methodMu.RLock()
	server := c.methodServer
	c.methodMu.RUnlock()
	if server == nil || server.client == nil {
		return
	}
	_ = server.client.Notify(context.Background(), req.Method, req.Params)
}

func handleCallMethodExecution(stream net.Conn, agentClient *AgentClient, call msg.CallMethodRequest) {
	if call.IsNotification {
		agentClient.SendNotification(call)
		return
	}
	response := agentClient.CallMethod(call)
	_ = msg.WriteMessage(stream, &msg.CallMethodResponse{Response: response})
}

// handleCallMethodBatchExecution forwards each item in the batch to the method
// server sequentially — single requests via CallMethod (which returns a
// response), notifications via SendNotification (fire-and-forget). Responses
// are collected in order and sent back as one CallMethodBatchResponse. The
// agent is a simple forwarder; concurrency comes from multiple concurrent
// callers (different yamux streams), not from within a single batch.
func handleCallMethodBatchExecution(stream net.Conn, agentClient *AgentClient, batch msg.CallMethodBatchRequest) {
	var responses []methods.JSONRPCResponse
	for _, item := range batch.Items {
		callReq := msg.CallMethodRequest{
			Method: item.Method,
			Params: item.Params,
			ID:     item.ID,
		}
		if item.IsNotification {
			agentClient.SendNotification(callReq)
			continue
		}
		resp := agentClient.CallMethod(callReq)
		responses = append(responses, resp)
	}
	_ = msg.WriteMessage(stream, &msg.CallMethodBatchResponse{Responses: responses})
}

// acquire reserves an in-flight slot, bailing out when ctx is cancelled (which
// also carries the per-call timeout) or the process has exited. The returned
// release func must be called when the caller is done with the slot. In
// concurrent mode semaphore is nil and this is a no-op.
func (s *methodServerProcess) acquire(ctx context.Context) (func(), error) {
	if s.semaphore == nil {
		return func() {}, nil
	}
	select {
	case s.semaphore <- struct{}{}:
		return func() {
			<-s.semaphore
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.closed:
		return nil, errors.New("method server unavailable")
	}
}

func (s *methodServerProcess) markClosed() {
	s.closeOnce.Do(func() { close(s.closed) })
}

// shutdown stops the server. For a real process, closing the client closes
// stdin (signalling the child to exit), waits up to the jsonrpc shutdown
// timeout, then kills if needed; the OnExit hook runs as the child is reaped.
func (s *methodServerProcess) shutdown() {
	s.markClosed()
	if s.client != nil {
		_ = s.client.Close()
	}
}

// methodError is a small helper for the JSON-RPC error responses CallMethod
// returns for non-round-trip failures.
func methodError(id any, code int, message string) methods.JSONRPCResponse {
	return methods.JSONRPCResponse{
		JSONRPC: "2.0",
		Error:   &methods.JSONRPCError{Code: code, Message: message},
		ID:      id,
	}
}

// toJSONResult decodes a raw JSON-RPC result into the any that
// methods.JSONRPCResponse.Result carries. Absent/null results yield nil. This
// matches the previous behaviour, which decoded the subprocess response into a
// methods.JSONRPCResponse directly.
func toJSONResult(raw json.RawMessage) any {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

// stderrLogWriter is an io.Writer fed to jsonrpc.WithStderr. It buffers child
// stderr bytes and emits one SendLogMessage per newline-delimited line, the
// same shape the previous pumpStderr produced. flush emits any trailing
// partial line and is called from the OnExit hook once the child has exited.
type stderrLogWriter struct {
	c   *AgentClient
	mu  sync.Mutex
	buf []byte
}

func (w *stderrLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		_ = w.c.SendLogMessage("knot_method_server", msg.LogLevelInfo, line)
	}
	return len(p), nil
}

func (w *stderrLogWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(bytes.TrimSpace(w.buf)) == 0 {
		w.buf = nil
		return
	}
	_ = w.c.SendLogMessage("knot_method_server", msg.LogLevelInfo, string(w.buf))
	w.buf = nil
}
