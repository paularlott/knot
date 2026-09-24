package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	knotmcp "github.com/paularlott/knot/internal/mcp"
	"github.com/paularlott/knot/internal/middleware"

	"github.com/paularlott/mcp"
)

// The api/chat/tools endpoints are scriptling's knot.mcp transport and must
// expose the web chat's tool view: knot's own tools plus federated remote
// servers (the chat server's registrations), never the public /mcp
// endpoint's native-only view.
func TestChatToolsEndpointsExposeChatToolView(t *testing.T) {
	// A fake remote MCP server exposing one tool.
	remote := mcp.NewServer("remote-test", "1.0.0")
	remote.RegisterTool(mcp.NewTool("remote_echo", "echoes its input"), func(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
		return mcp.NewToolResponseAuto("ok"), nil
	})
	remoteSrv := httptest.NewServer(http.HandlerFunc(remote.HandleRequest))
	defer remoteSrv.Close()

	// The internal (chat) server, federating the remote under namespace "fed".
	routes := &http.ServeMux{}
	chatServer := knotmcp.InitializeMCPServer(routes, false, &config.MCPConfig{
		RemoteServers: []config.MCPRemoteServerConfig{{
			Namespace:      "fed",
			URL:            remoteSrv.URL,
			Token:          "test-token",
			ToolVisibility: "native",
		}},
	})

	// The same middleware stack the routes get in command/server.go: the
	// chat server plus per-user providers in context. The user-remote
	// provider needs the database, so the test stack carries the method
	// provider; config-level federation is what the remote tool proves.
	user := &model.User{Id: "u1", Username: "t"}
	stack := func(next http.Handler) http.Handler {
		return middleware.MCPServerContext(chatServer, func(ctx context.Context, u *model.User) []mcp.ToolProvider {
			return []mcp.ToolProvider{knotmcp.NewMethodToolsProvider(u)}
		})(next)
	}

	// GET api/chat/tools lists the federated remote tool.
	listReq := httptest.NewRequest(http.MethodGet, "/api/chat/tools", nil)
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), "user", user))
	listRec := httptest.NewRecorder()
	stack(http.HandlerFunc(HandleListTools)).ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), "fed"+mcp.DefaultNamespaceSeparator+"remote_echo") {
		t.Fatalf("tool listing must include the federated remote tool: %s", listRec.Body.String())
	}

	// POST api/chat/tools/call dispatches to it.
	callReq := httptest.NewRequest(http.MethodPost, "/api/chat/tools/call", strings.NewReader(`{"name":"fed__remote_echo","arguments":{}}`))
	callReq.Header.Set("Content-Type", "application/json")
	callReq = callReq.WithContext(context.WithValue(callReq.Context(), "user", user))
	callRec := httptest.NewRecorder()
	stack(http.HandlerFunc(HandleCallTool)).ServeHTTP(callRec, callReq)

	if callRec.Code != http.StatusOK {
		t.Fatalf("call status = %d, want 200: %s", callRec.Code, callRec.Body.String())
	}
}
