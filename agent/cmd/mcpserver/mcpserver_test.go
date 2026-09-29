package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/paularlott/knot/internal/config"

	mcp "github.com/paularlott/mcp"
)

// fakeUpstream is a knot-like /mcp endpoint: bearer auth, a native tool and a
// discoverable tool, and a record of the headers it was called with.
type fakeUpstream struct {
	token        string
	showAllSeen  bool
	authSeen     string
	requestCount int
	mu           sync.Mutex
}

func newFakeUpstream(t *testing.T) (*fakeUpstream, *httptest.Server) {
	t.Helper()

	up := &fakeUpstream{token: "test-token"}

	server := mcp.NewServer("knot-test", "1.0")
	server.RegisterTool(
		mcp.NewTool("space_list", "Lists spaces."),
		func(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
			return mcp.NewToolResponseText("native ok"), nil
		},
	)
	server.RegisterTool(
		mcp.NewTool("search_docs", "Searches docs.").Discoverable("docs"),
		func(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
			return mcp.NewToolResponseText("discoverable ok"), nil
		},
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up.mu.Lock()
		up.requestCount++
		up.authSeen = r.Header.Get("Authorization")
		if strings.EqualFold(r.Header.Get(mcp.ShowAllHeader), "true") {
			up.showAllSeen = true
		}
		up.mu.Unlock()

		if r.Header.Get("Authorization") != "Bearer "+up.token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Mirror knot's /mcp endpoint: show-all mode is taken from the
		// request header/query before the server sees the request.
		ctx := mcp.WithShowAllFromRequest(r.Context(), r)
		server.HandleRequest(w, r.WithContext(ctx))
	}))
	t.Cleanup(ts.Close)

	return up, ts
}

// rpcCall writes one JSON-RPC request and returns the matching response
// object, skipping notifications (which carry no id).
func rpcCall(t *testing.T, in io.Writer, out *bufio.Reader, id int, method string, params map[string]interface{}) map[string]interface{} {
	t.Helper()

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if _, err := in.Write(append(data, '\n')); err != nil {
		t.Fatalf("write request: %v", err)
	}

	for {
		line, err := out.ReadString('\n')
		if err != nil {
			t.Fatalf("reading response for %s: %v", method, err)
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("unmarshal %q: %v", line, err)
		}
		if _, isNotification := msg["id"]; !isNotification {
			continue
		}
		if msg["id"] == float64(id) {
			return msg
		}
	}
}

func TestMCPServerProxy(t *testing.T) {
	up, ts := newFakeUpstream(t)
	cfg := config.NewServerAddr(ts.URL, "test-token")

	server, client, err := buildMCPServer(cfg, false, true)
	if err != nil {
		t.Fatalf("buildMCPServer: %v", err)
	}
	defer client.Close()

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- server.ServeStream(context.Background(), stdinR, stdoutW)
	}()

	out := bufio.NewReader(stdoutR)

	resp := rpcCall(t, stdinW, out, 1, "initialize", map[string]interface{}{
		"protocolVersion": mcp.MCPProtocolVersionLatest,
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "test", "version": "1.0"},
	})
	if resp["error"] != nil {
		t.Fatalf("initialize failed: %v", resp["error"])
	}

	// notifications/initialized — no response expected
	if _, err := stdinW.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")); err != nil {
		t.Fatalf("write initialized: %v", err)
	}

	resp = rpcCall(t, stdinW, out, 2, "tools/list", map[string]interface{}{})
	if resp["error"] != nil {
		t.Fatalf("tools/list failed: %v", resp["error"])
	}
	result := resp["result"].(map[string]interface{})
	tools := result["tools"].([]interface{})
	names := map[string]bool{}
	for _, tl := range tools {
		tool := tl.(map[string]interface{})
		names[tool["name"].(string)] = true
		if strings.Contains(tool["name"].(string), "knot__") {
			t.Errorf("tool %q carries a namespace prefix", tool["name"])
		}
	}
	if !names["space_list"] {
		t.Errorf("native tool missing from tools/list: %v", names)
	}
	if !names["search_docs"] {
		t.Errorf("discoverable tool missing with --show-all: %v", names)
	}

	resp = rpcCall(t, stdinW, out, 3, "tools/call", map[string]interface{}{
		"name":      "space_list",
		"arguments": map[string]interface{}{},
	})
	if resp["error"] != nil {
		t.Fatalf("tools/call failed: %v", resp["error"])
	}
	callRes := resp["result"].(map[string]interface{})
	content := callRes["content"].([]interface{})[0].(map[string]interface{})
	if content["text"] != "native ok" {
		t.Errorf("unexpected tool result: %v", content)
	}

	// The upstream enforced bearer auth and saw the show-all header.
	up.mu.Lock()
	auth := up.authSeen
	showAll := up.showAllSeen
	requests := up.requestCount
	up.mu.Unlock()
	if requests == 0 || auth != "Bearer test-token" {
		t.Errorf("upstream auth not as expected: requests=%d auth=%q", requests, auth)
	}
	if !showAll {
		t.Error("X-MCP-Show-All header never reached the upstream")
	}

	// Stdin EOF ends the server.
	stdinW.Close()
	<-done
}

func TestMCPServerProxyWithoutShowAll(t *testing.T) {
	up, ts := newFakeUpstream(t)
	cfg := config.NewServerAddr(ts.URL, "test-token")

	server, client, err := buildMCPServer(cfg, false, false)
	if err != nil {
		t.Fatalf("buildMCPServer: %v", err)
	}
	defer client.Close()

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- server.ServeStream(context.Background(), stdinR, stdoutW)
	}()
	out := bufio.NewReader(stdoutR)

	rpcCall(t, stdinW, out, 1, "initialize", map[string]interface{}{
		"protocolVersion": mcp.MCPProtocolVersionLatest,
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "test", "version": "1.0"},
	})
	resp := rpcCall(t, stdinW, out, 2, "tools/list", map[string]interface{}{})
	if resp["error"] != nil {
		t.Fatalf("tools/list failed: %v", resp["error"])
	}
	result := resp["result"].(map[string]interface{})
	for _, tl := range result["tools"].([]interface{}) {
		if tl.(map[string]interface{})["name"] == "search_docs" {
			t.Error("discoverable tool listed without --show-all")
		}
	}

	up.mu.Lock()
	showAll := up.showAllSeen
	up.mu.Unlock()
	if showAll {
		t.Error("X-MCP-Show-All header reached the upstream without --show-all")
	}

	stdinW.Close()
	<-done
}
