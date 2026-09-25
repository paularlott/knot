package middleware

import (
	"context"
	"net/http"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/mcp"
)

// ScriptToolsProvider is a callback function that returns the request-scoped
// ToolProviders for script tools. Returned as a slice — not merged into one
// mcp.NewMultiProvider — so each stays individually visible to
// mcp.GetToolProviders(ctx): a provider that also implements an optional
// interface (e.g. lmchatkit.SourcedToolProvider) needs to be discoverable by
// type assertion, which a MultiProvider wrapper would hide.
type ScriptToolsProvider func(ctx context.Context, user *model.User) []mcp.ToolProvider

// SkillProviders is a callback function that returns the request-scoped
// skills surface for a user: entries for skills/list and skills/get, plus
// the resource reads those entries point at (the same object usually, hence
// the two returns). Either may be nil when the user has no skills.
type SkillProviders func(ctx context.Context, user *model.User) (skills mcp.SkillProvider, resources mcp.ResourceProvider)

// MCPServerContext adds the MCP server and request-scoped script tools and
// skills to the request context
func MCPServerContext(mcpServer *mcp.Server, scriptToolsProvider ScriptToolsProvider, skillProviders SkillProviders) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Add MCP server to context
			ctx = context.WithValue(ctx, "mcp", mcpServer)

			user, _ := ctx.Value("user").(*model.User)

			// Add script tools as request-scoped providers
			if scriptToolsProvider != nil {
				log.Debug("MCPServerContext: user from context", "ok", user != nil, "user", user)
				if user != nil {
					if providers := scriptToolsProvider(ctx, user); len(providers) > 0 {
						log.Debug("MCPServerContext: adding script tools providers to context", "user", user.Username, "count", len(providers))
						ctx = mcp.WithToolProviders(ctx, providers...)
					} else {
						log.Debug("MCPServerContext: scriptToolsProvider returned none", "user", user.Username, "has_ExecuteScripts", user.HasPermission(model.PermissionExecuteScripts), "has_ExecuteOwnScripts", user.HasPermission(model.PermissionExecuteOwnScripts))
					}
				} else {
					log.Debug("MCPServerContext: user not found in context")
				}
			}

			// Add the user's skills: skills/list and skills/get entries, and
			// the skill:// resources those entries point at.
			if skillProviders != nil && user != nil {
				if skills, resources := skillProviders(ctx, user); skills != nil {
					ctx = mcp.WithSkillProviders(ctx, skills)
					if resources != nil {
						ctx = mcp.WithResourceProviders(ctx, resources)
					}
				}
			}

			// Check for show-all mode (MCP chaining)
			if mcp.GetShowAllFromRequest(r) {
				ctx = mcp.WithShowAllTools(ctx)
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
