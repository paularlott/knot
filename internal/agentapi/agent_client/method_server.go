package agent_client

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/methods"
)

const (
	defaultMethodTimeoutSeconds = 30
)

// methodServerProcess owns the long-running stdio method server. Requests are
// written to stdin and responses are read from stdout by a dedicated reader
// goroutine. Concurrent mode allows many requests to be in flight at once;
// serial mode allows one. Responses are correlated with callers by an internal
// wire id so the caller's original JSON-RPC id never has to be unique.
type methodServerProcess struct {
	reg *methods.Registration

	cmd   *exec.Cmd
	stdin io.WriteCloser

	// writeLock serializes writes to stdin. It is held only for the duration of
	// a single Write call, never across the round trip.
	writeLock sync.Mutex

	// semaphore caps the number of in-flight requests. nil means unlimited
	// (concurrent mode). A buffered channel of size 1 enforces serial mode.
	semaphore chan struct{}

	// pending maps the internal wire id to the waiting caller.
	pendingMu sync.Mutex
	pending   map[int64]*pendingCall

	nextID atomic.Int64

	// readerDone is closed when the stdout reader goroutine exits.
	readerDone chan struct{}
	// closed is closed once the process is shutting down or the reader has
	// exited. Pending and future callers fail with closeErr.
	closed    chan struct{}
	closeOnce sync.Once
	closeMu   sync.Mutex
	closeErr  error
}

// pendingCall is a single in-flight method call waiting for a response.
type pendingCall struct {
	callerID   any
	responseCh chan *methods.JSONRPCResponse
}

func (c *AgentClient) RegisterMethods(reg *methods.Registration) error {
	if reg == nil {
		return fmt.Errorf("registration is required")
	}
	if reg.Server.Command == "" {
		return fmt.Errorf("server command is required")
	}
	if err := c.startMethodServer(reg); err != nil {
		return err
	}
	if err := c.publishMethods(reg); err != nil {
		c.stopMethodServer()
		return err
	}

	// Stash so we can re-publish after any knot-server reconnect (the server's
	// registry is in-memory and lost on restart).
	c.lastRegMu.Lock()
	c.lastReg = reg
	c.lastRegMu.Unlock()

	return nil
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

func (c *AgentClient) startMethodServer(reg *methods.Registration) error {
	c.stopMethodServer()

	args := reg.Server.Args
	cmd := exec.Command(reg.Server.Command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	server := &methodServerProcess{
		reg:        reg,
		cmd:        cmd,
		stdin:      stdin,
		pending:    make(map[int64]*pendingCall),
		readerDone: make(chan struct{}),
		closed:     make(chan struct{}),
	}
	if reg.Server.Mode == methods.ModeSerial {
		server.semaphore = make(chan struct{}, 1)
	}

	c.methodMu.Lock()
	c.methodServer = server
	c.methodMu.Unlock()

	go pumpStderr(stderr)
	go server.readLoop(stdout)
	go func() {
		err := cmd.Wait()
		server.shutdown()

		// Clear the method server reference so CallMethod returns
		// "no method server available" instead of routing to a dead process.
		c.methodMu.Lock()
		if c.methodServer == server {
			c.methodServer = nil
		}
		c.methodMu.Unlock()

		// Tell the knot server to remove our methods from the registry.
		// Without this, the methods stay discoverable even though the
		// backing process is dead — callers would get "no method server
		// available" on every call until the session drops.
		c.unregisterMethods()

		// Clear the stashed registration so a subsequent server reconnect
		// doesn't republish methods whose backing process has exited.
		c.lastRegMu.Lock()
		c.lastReg = nil
		c.lastRegMu.Unlock()

		if err != nil {
			log.WithError(err).Warn("method server exited")
		} else {
			log.Info("method server exited")
		}
	}()

	return nil
}

func (c *AgentClient) stopMethodServer() {
	c.methodMu.Lock()
	server := c.methodServer
	c.methodServer = nil
	c.methodMu.Unlock()

	if server == nil {
		return
	}
	server.shutdown()
	if server.cmd != nil && server.cmd.Process != nil {
		_ = server.stdin.Close()
		_ = server.cmd.Process.Kill()
	}
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
		var response msg.RegisterMethodsResponse
		if err == nil {
			err = msg.ReadMessage(conn, &response)
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
// time. The caller's id is preserved on the returned response.
func (c *AgentClient) CallMethod(req msg.CallMethodRequest) methods.JSONRPCResponse {
	c.methodMu.RLock()
	server := c.methodServer
	c.methodMu.RUnlock()
	if server == nil {
		return methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32000, Message: "no method server available"},
			ID:      req.ID,
		}
	}

	timeout := server.reg.Server.Timeout
	if timeout <= 0 {
		timeout = defaultMethodTimeoutSeconds
	}
	timer := time.NewTimer(time.Duration(timeout) * time.Second)
	defer timer.Stop()

	// Acquire a concurrency slot. For serial mode this blocks until the prior
	// call finishes. For concurrent mode semaphore is nil and this is a no-op.
	release, acquireErr := server.acquire(timer.C)
	if acquireErr != nil {
		return methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32000, Message: acquireErr.Error()},
			ID:      req.ID,
		}
	}
	defer release()

	wireID, call, ok := server.registerPending(req.ID)
	if !ok {
		return methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32000, Message: "method server unavailable"},
			ID:      req.ID,
		}
	}

	wireReq := methods.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  req.Method,
		Params:  req.Params,
		ID:      wireID,
	}
	data, err := json.Marshal(wireReq)
	if err != nil {
		server.removePending(wireID)
		return methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32603, Message: err.Error()},
			ID:      req.ID,
		}
	}
	data = append(data, '\n')

	if err := server.write(data); err != nil {
		server.removePending(wireID)
		return methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32000, Message: err.Error()},
			ID:      req.ID,
		}
	}

	select {
	case resp := <-call.responseCh:
		if resp == nil {
			return methods.JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   &methods.JSONRPCError{Code: -32000, Message: "method server closed before responding"},
				ID:      req.ID,
			}
		}
		resp.ID = req.ID
		if resp.JSONRPC == "" {
			resp.JSONRPC = "2.0"
		}
		c.methodCallsTotal.Add(1)
		return *resp
	case <-timer.C:
		server.removePending(wireID)
		return methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32000, Message: "method server timeout"},
			ID:      req.ID,
		}
	case <-server.closed:
		server.removePending(wireID)
		server.closeMu.Lock()
		err := server.closeErr
		server.closeMu.Unlock()
		msg := "method server unavailable"
		if err != nil {
			msg = err.Error()
		}
		return methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32000, Message: msg},
			ID:      req.ID,
		}
	}
}

// SendNotification forwards a JSON-RPC notification (no id) to the method
// server. Unlike CallMethod it writes to stdin and returns immediately — no
// response is expected. If the method server happens to write a response
// anyway (non-standard), the reader goroutine drops it since no pending call
// was registered for that wire id.
func (c *AgentClient) SendNotification(req msg.CallMethodRequest) {
	c.methodMu.RLock()
	server := c.methodServer
	c.methodMu.RUnlock()
	if server == nil {
		return
	}

	// Build a true JSON-RPC notification (no id). The method server receives
	// the same message shape it would for any notification.
	notification := methods.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  req.Method,
		Params:  req.Params,
		// No ID — this is a notification per JSON-RPC 2.0 spec.
	}
	data, err := json.Marshal(notification)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_ = server.write(data)
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

// acquire reserves an in-flight slot, blocking until one is available, the
// timeout fires, or the server is closed. The returned release func must be
// called when the caller is done with the slot.
func (s *methodServerProcess) acquire(timeout <-chan time.Time) (func(), error) {
	if s.semaphore == nil {
		return func() {}, nil
	}
	select {
	case s.semaphore <- struct{}{}:
		return func() {
			<-s.semaphore
		}, nil
	case <-timeout:
		return nil, fmt.Errorf("method server timeout")
	case <-s.closed:
		return nil, s.closeErrOr("method server unavailable")
	}
}

func (s *methodServerProcess) registerPending(callerID any) (int64, *pendingCall, bool) {
	if s.isClosed() {
		return 0, nil, false
	}
	wireID := s.nextID.Add(1)
	call := &pendingCall{
		callerID:   callerID,
		responseCh: make(chan *methods.JSONRPCResponse, 1),
	}
	s.pendingMu.Lock()
	if s.isClosedLocked() {
		s.pendingMu.Unlock()
		return 0, nil, false
	}
	s.pending[wireID] = call
	s.pendingMu.Unlock()
	return wireID, call, true
}

func (s *methodServerProcess) removePending(wireID int64) {
	s.pendingMu.Lock()
	delete(s.pending, wireID)
	s.pendingMu.Unlock()
}

// write writes one framed JSON-RPC request to stdin. It is safe to call
// concurrently; only the write itself is serialized.
func (s *methodServerProcess) write(data []byte) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()
	if s.isClosed() {
		return s.closeErrOr("method server unavailable")
	}
	if _, err := s.stdin.Write(data); err != nil {
		s.shutdownWith(fmt.Errorf("method server stdin write failed: %w", err))
		return err
	}
	return nil
}

// readLoop owns stdout. It decodes JSON-RPC responses one at a time and
// dispatches them to the waiting caller by wire id. When stdout closes or
// decoding fails, the loop marks the server shut down and fails every pending
// caller.
func (s *methodServerProcess) readLoop(stdout io.Reader) {
	defer close(s.readerDone)
	decoder := json.NewDecoder(stdout)
	for {
		var resp methods.JSONRPCResponse
		if err := decoder.Decode(&resp); err != nil {
			if err != io.EOF && !isClosedErr(err) {
				log.WithError(err).Warn("method server stdout decode error")
			}
			s.shutdownWith(fmt.Errorf("method server stdout closed: %w", err))
			return
		}
		wireID, ok := extractWireID(resp.ID)
		if !ok {
			continue
		}
		s.pendingMu.Lock()
		call, found := s.pending[wireID]
		if found {
			delete(s.pending, wireID)
		}
		s.pendingMu.Unlock()
		if !found {
			continue
		}
		select {
		case call.responseCh <- &resp:
		case <-s.closed:
		}
	}
}

func extractWireID(id any) (int64, bool) {
	switch v := id.(type) {
	case float64:
		return int64(v), true
	case int:
		return int64(v), true
	case int64:
		return v, true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// shutdown marks the server closed without overwriting an existing error.
// Subsequent calls are no-ops.
func (s *methodServerProcess) shutdown() {
	s.shutdownWith(nil)
}

func (s *methodServerProcess) shutdownWith(err error) {
	s.closeOnce.Do(func() {
		if err != nil {
			s.closeMu.Lock()
			s.closeErr = err
			s.closeMu.Unlock()
		}
		close(s.closed)
	})
	s.failPending()
}

// failPending delivers a nil response (interpreted as "server closed") to
// every waiting caller. It is safe to call multiple times.
func (s *methodServerProcess) failPending() {
	s.pendingMu.Lock()
	pending := s.pending
	s.pending = make(map[int64]*pendingCall)
	s.pendingMu.Unlock()
	for _, call := range pending {
		select {
		case call.responseCh <- nil:
		default:
		}
	}
}

func (s *methodServerProcess) isClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *methodServerProcess) isClosedLocked() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *methodServerProcess) closeErrOr(defaultMsg string) error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closeErr != nil {
		return s.closeErr
	}
	return errors.New(defaultMsg)
}

func pumpStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		log.Info("method server", "line", scanner.Text())
	}
}

func isClosedErr(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return true
	}
	return false
}
