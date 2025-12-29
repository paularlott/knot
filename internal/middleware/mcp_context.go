package middleware

import (
	"context"
	"net/http"

	"github.com/paularlott/mcp"
	"github.com/paularlott/mcp/discovery"
)

// ScriptToolsProvider is a callback function that returns a discovery.ToolRegistry with script tools for a user
type ScriptToolsProvider func(ctx context.Context) *discovery.ToolRegistry

// MCPServerContext adds the MCP server and request-scoped script tools to the request context
func MCPServerContext(mcpServer *mcp.Server, scriptToolsProvider ScriptToolsProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Add MCP server to context
			ctx = context.WithValue(ctx, "mcp", mcpServer)

			// Add script tools as request-scoped provider for tool discovery
			if scriptToolsProvider != nil {
				if scriptRegistry := scriptToolsProvider(ctx); scriptRegistry != nil {
					ctx = discovery.WithRequestProviders(ctx, scriptRegistry)
				}
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// MCPServerContextSimple adds only the MCP server to the request context (without script tools)
// Use this when you don't need access to script tools via tool_search
func MCPServerContextSimple(mcpServer *mcp.Server) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add MCP server to context
			ctx := context.WithValue(r.Context(), "mcp", mcpServer)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}