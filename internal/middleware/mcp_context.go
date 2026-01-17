package middleware

import (
	"context"
	"net/http"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/mcp"
)

// ScriptToolsProvider is a callback function that returns a ToolProvider for script tools
type ScriptToolsProvider func(ctx context.Context, user *model.User) mcp.ToolProvider

// MCPServerContext adds the MCP server and request-scoped script tools to the request context
func MCPServerContext(mcpServer *mcp.Server, scriptToolsProvider ScriptToolsProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Add MCP server to context
			ctx = context.WithValue(ctx, "mcp", mcpServer)

			// Add script tools as request-scoped provider in force ondemand mode
			if scriptToolsProvider != nil {
				user, ok := ctx.Value("user").(*model.User)
				if ok && user != nil {
					if provider := scriptToolsProvider(ctx, user); provider != nil {
						ctx = mcp.WithForceOnDemandMode(ctx, provider)
					} else {
						ctx = mcp.WithForceOnDemandMode(ctx)
					}
				} else {
					ctx = mcp.WithForceOnDemandMode(ctx)
				}
			} else {
				ctx = mcp.WithForceOnDemandMode(ctx)
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