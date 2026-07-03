package agent_client

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/paularlott/jsonrpc"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/methods"
)

// newFakeMethodServer starts a goroutine that emulates a stdio JSON-RPC method
// server. It returns the agent-side stdin (writes go to the server) and stdout
// (responses come back). Each request is handled in its own goroutine so
// concurrent inflight can be observed; stdout writes are serialized.
func newFakeMethodServer(t *testing.T, handler func(req methods.JSONRPCRequest) methods.JSONRPCResponse) (io.WriteCloser, io.ReadCloser) {
	t.Helper()
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	go func() {
		defer stdinRead.Close()
		defer stdoutWrite.Close()
		decoder := json.NewDecoder(stdinRead)
		encoder := json.NewEncoder(stdoutWrite)
		var writeMu sync.Mutex
		var wg sync.WaitGroup
		for {
			var req methods.JSONRPCRequest
			if err := decoder.Decode(&req); err != nil {
				break
			}
			wg.Add(1)
			go func(req methods.JSONRPCRequest) {
				defer wg.Done()
				resp := handler(req)
				writeMu.Lock()
				_ = encoder.Encode(&resp)
				writeMu.Unlock()
			}(req)
		}
		wg.Wait()
	}()
	return stdinWrite, stdoutRead
}

// attachServer wires a methodServerProcess to a jsonrpc stream transport backed
// by the given pipes (no real os/exec process), so CallMethod can be exercised
// in isolation.
func attachServer(client *AgentClient, reg *methods.Registration, stdin io.WriteCloser, stdout io.ReadCloser) *methodServerProcess {
	transport := jsonrpc.NewStreamTransport(stdout, stdin)
	server := &methodServerProcess{
		reg:       reg,
		client:    jsonrpc.NewClient(transport),
		transport: transport,
		closed:    make(chan struct{}),
	}
	if reg.Server.Mode == methods.ModeSerial {
		server.semaphore = make(chan struct{}, 1)
	}
	client.methodMu.Lock()
	client.methodServer = server
	client.methodMu.Unlock()
	return server
}

func TestCallMethodConcurrentAllowsManyInflight(t *testing.T) {
	var mu sync.Mutex
	inflight, maxInflight := 0, 0
	handler := func(req methods.JSONRPCRequest) methods.JSONRPCResponse {
		mu.Lock()
		inflight++
		if inflight > maxInflight {
			maxInflight = inflight
		}
		mu.Unlock()
		time.Sleep(80 * time.Millisecond)
		mu.Lock()
		inflight--
		mu.Unlock()
		return methods.JSONRPCResponse{JSONRPC: "2.0", Result: "ok", ID: req.ID}
	}

	stdin, stdout := newFakeMethodServer(t, handler)
	client := NewAgentClient("test:0", "space")
	reg := &methods.Registration{Server: methods.ServerConfig{Mode: methods.ModeConcurrent, Timeout: 5}}
	server := attachServer(client, reg, stdin, stdout)
	defer server.shutdown()
	defer stdin.Close()

	const N = 8
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp := client.CallMethod(msg.CallMethodRequest{Method: "ping", ID: i + 100})
			if resp.Error != nil {
				t.Errorf("call %d: unexpected error: %v", i, resp.Error)
				return
			}
			if resp.ID != i+100 {
				t.Errorf("call %d: response id mismatch: got %v want %d", i, resp.ID, i+100)
			}
		}(i)
	}
	wg.Wait()

	if maxInflight < 2 {
		t.Errorf("expected concurrent inflight >= 2, got max %d", maxInflight)
	}
}

func TestCallMethodSerialAllowsOnlyOneInflight(t *testing.T) {
	var mu sync.Mutex
	inflight, maxInflight := 0, 0
	handler := func(req methods.JSONRPCRequest) methods.JSONRPCResponse {
		mu.Lock()
		inflight++
		if inflight > maxInflight {
			maxInflight = inflight
		}
		mu.Unlock()
		time.Sleep(40 * time.Millisecond)
		mu.Lock()
		inflight--
		mu.Unlock()
		return methods.JSONRPCResponse{JSONRPC: "2.0", Result: "ok", ID: req.ID}
	}

	stdin, stdout := newFakeMethodServer(t, handler)
	client := NewAgentClient("test:0", "space")
	reg := &methods.Registration{Server: methods.ServerConfig{Mode: methods.ModeSerial, Timeout: 5}}
	server := attachServer(client, reg, stdin, stdout)
	defer server.shutdown()
	defer stdin.Close()

	const N = 6
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp := client.CallMethod(msg.CallMethodRequest{Method: "ping", ID: i})
			if resp.Error != nil {
				t.Errorf("call %d: %v", i, resp.Error)
			}
		}(i)
	}
	wg.Wait()

	if maxInflight != 1 {
		t.Errorf("expected serial inflight == 1, got %d", maxInflight)
	}
}

func TestCallMethodPreservesNonNumericCallerID(t *testing.T) {
	handler := func(req methods.JSONRPCRequest) methods.JSONRPCResponse {
		return methods.JSONRPCResponse{JSONRPC: "2.0", Result: "ok", ID: req.ID}
	}
	stdin, stdout := newFakeMethodServer(t, handler)
	client := NewAgentClient("test:0", "space")
	reg := &methods.Registration{Server: methods.ServerConfig{Mode: methods.ModeConcurrent, Timeout: 5}}
	server := attachServer(client, reg, stdin, stdout)
	defer server.shutdown()
	defer stdin.Close()

	for _, callerID := range []any{"caller-1", "abc-xyz", nil} {
		resp := client.CallMethod(msg.CallMethodRequest{Method: "echo", ID: callerID})
		if resp.Error != nil {
			t.Fatalf("unexpected error for id %v: %v", callerID, resp.Error)
		}
		if resp.ID != callerID {
			t.Errorf("caller id not preserved: got %v want %v", resp.ID, callerID)
		}
	}
}

func TestCallMethodTimeout(t *testing.T) {
	handler := func(req methods.JSONRPCRequest) methods.JSONRPCResponse {
		time.Sleep(2 * time.Second)
		return methods.JSONRPCResponse{JSONRPC: "2.0", Result: "ok", ID: req.ID}
	}
	stdin, stdout := newFakeMethodServer(t, handler)
	client := NewAgentClient("test:0", "space")
	reg := &methods.Registration{Server: methods.ServerConfig{Mode: methods.ModeConcurrent, Timeout: 1}}
	server := attachServer(client, reg, stdin, stdout)
	defer server.shutdown()
	defer stdin.Close()

	start := time.Now()
	resp := client.CallMethod(msg.CallMethodRequest{Method: "slow", ID: "abc"})
	elapsed := time.Since(start)
	if resp.Error == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed >= 2*time.Second {
		t.Errorf("timeout did not fire early enough: %v", elapsed)
	}
}

func TestCallMethodFailsAfterShutdown(t *testing.T) {
	handler := func(req methods.JSONRPCRequest) methods.JSONRPCResponse {
		return methods.JSONRPCResponse{JSONRPC: "2.0", Result: "ok", ID: req.ID}
	}
	stdin, stdout := newFakeMethodServer(t, handler)
	client := NewAgentClient("test:0", "space")
	reg := &methods.Registration{Server: methods.ServerConfig{Mode: methods.ModeConcurrent, Timeout: 5}}
	server := attachServer(client, reg, stdin, stdout)
	defer stdin.Close()

	server.shutdown()
	resp := client.CallMethod(msg.CallMethodRequest{Method: "ping", ID: 1})
	if resp.Error == nil {
		t.Fatal("expected error after shutdown")
	}
}
