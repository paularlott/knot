package middleware

import (
	"context"
	"net/http"

	"github.com/paularlott/mcp"
)

// MCPServerContext adds the MCP server to the request context
func MCPServerContext(mcpServer *mcp.Server) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add MCP server to context
			ctx := context.WithValue(r.Context(), "mcp", mcpServer)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}