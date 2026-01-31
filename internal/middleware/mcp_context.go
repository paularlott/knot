package middleware

import (
	"context"
	"net/http"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
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

			// Add script tools as request-scoped provider in force on-demand mode
			if scriptToolsProvider != nil {
				user, ok := ctx.Value("user").(*model.User)
				log.Debug("MCPServerContext: user from context", "ok", ok, "user", user)
				if ok && user != nil {
					if provider := scriptToolsProvider(ctx, user); provider != nil {
						log.Debug("MCPServerContext: adding script tools provider to context", "user", user.Username)
						ctx = mcp.WithForceOnDemandMode(ctx, provider)
					} else {
						log.Debug("MCPServerContext: scriptToolsProvider returned nil", "user", user.Username, "has_ExecuteScripts", user.HasPermission(model.PermissionExecuteScripts), "has_ExecuteOwnScripts", user.HasPermission(model.PermissionExecuteOwnScripts))
						ctx = mcp.WithForceOnDemandMode(ctx)
					}
				} else {
					log.Debug("MCPServerContext: user not found in context")
					ctx = mcp.WithForceOnDemandMode(ctx)
				}
			} else {
				ctx = mcp.WithForceOnDemandMode(ctx)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// HandlerToHandlerFunc converts an http.Handler to an http.HandlerFunc
// This is useful when you need to use an http.Handler with middleware that expects http.HandlerFunc
func HandlerToHandlerFunc(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	}
}