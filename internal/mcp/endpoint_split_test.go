package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/paularlott/mcp"
)

// The public /mcp endpoint serves knot's own tools only. A configured remote
// server's tools must appear on the internal server InitializeMCPServer
// returns (web chat, OpenAI endpoints, scriptling's knot.mcp all resolve
// their tools through it) but never in the endpoint's tools/list — external
// MCP clients connect to a remote server directly, not through knot.
func TestPublicMCPEndpointExcludesRemoteServers(t *testing.T) {
	prev := config.GetServerConfig()
	// LeafNode skips the permission gate (checked against the role cache,
	// which needs the database); ApiAuth passes the context user through.
	config.SetServerConfig(&config.ServerConfig{LeafNode: true})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	// A fake remote MCP server exposing one tool.
	remote := mcp.NewServer("remote-test", "1.0.0")
	remote.RegisterTool(mcp.NewTool("remote_echo", "echoes its input"), func(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
		return mcp.NewToolResponseAuto("ok"), nil
	})
	remoteSrv := httptest.NewServer(http.HandlerFunc(remote.HandleRequest))
	defer remoteSrv.Close()

	cfg := &config.MCPConfig{
		RemoteServers: []config.MCPRemoteServerConfig{{
			Namespace:      "fed",
			URL:            remoteSrv.URL,
			Token:          "test-token",
			ToolVisibility: "native",
		}},
	}

	routes := &http.ServeMux{}
	chatServer := InitializeMCPServer(routes, true, cfg)

	// The internal server federates the remote tool...
	internalNames := chatServer.ListToolsWithContext(context.Background())
	hasRemote := false
	for _, tool := range internalNames {
		if strings.HasPrefix(tool.Name, "fed"+mcp.DefaultNamespaceSeparator) {
			hasRemote = true
		}
	}
	if !hasRemote {
		t.Fatalf("internal server should expose the remote server's tools for the chat path, got: %v", internalNames)
	}

	// ...but the public /mcp endpoint does not list it.
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "user", &model.User{Id: "u1", Username: "t"}))
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "remote_echo") {
		t.Fatalf("public /mcp tools/list must not include remote tools: %s", rec.Body.String())
	}
}
