// Package webchat adapts the github.com/paularlott/webchat library to knot's
// per-user, MCP-backed chat architecture.
//
// Knot uses webchat.StandardHost (same as llmrouter) with the OpenAI base URL
// pointing at knot's configured LLM endpoint. This is required because the
// MCP AI client's StreamChatCompletion SUPPRESSES tool-call delta chunks when
// MCP servers are present — webchat's manual approval flow needs those chunks
// to reach the frontend. StandardHost uses TranslateOpenAIStream on the raw
// HTTP response, bypassing the suppression.
//
// Per-user MCP tools are injected via the auth middleware (which wraps every
// webchat route): it sets up the tool provider in the request context so
// StandardHost.ListTools / CallTool resolve the user's tools through the MCP
// server's context-aware methods.
package webchat

import (
	"context"
	"net/http"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	internalmcp "github.com/paularlott/knot/internal/mcp"
	"github.com/paularlott/knot/internal/service"

	mcplib "github.com/paularlott/mcp"
	"github.com/paularlott/webchat"
)

// ScriptToolsProvider returns a per-user MCP tool provider (script tools +
// method tools), matching the MCPServerContext middleware logic.
type ScriptToolsProvider func(ctx context.Context, user *model.User) mcplib.ToolProvider

// NewHost builds a webchat.StandardHost configured for knot's LLM endpoint,
// MCP server, and single persona. The per-user tool provider is injected by
// AuthMiddleware (callers must wrap webchat's routes with it).
func NewHost(cfg config.ChatConfig, mcpServer *mcplib.Server, scriptToolsProvider ScriptToolsProvider) *webchat.StandardHost {
	// StandardHost.Complete appends "/v1/chat/completions" itself, so strip
	// a trailing /v1 from the configured BaseURL to avoid a doubled path
	// (e.g. http://host:1234/v1 → http://host:1234 + /v1/chat/completions).
	baseURL := strings.TrimSuffix(cfg.BaseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")

	return &webchat.StandardHost{
		ModelsFunc: func(ctx context.Context) ([]webchat.Model, error) {
			if cfg.Model == "" {
				return nil, nil
			}
			return []webchat.Model{{ID: cfg.Model}}, nil
		},
		OpenAIBaseURL: baseURL,
		OpenAIToken:   cfg.APIKey,
		MCPServer: func(ctx context.Context) *mcplib.Server {
			return mcpServer
		},
		SystemPromptAugmenter: func(ctx context.Context, current string) string {
			user := userFromCtx(ctx)
			if user == nil {
				return current
			}
			return current + internalmcp.BuildSkillsPrompt(user)
		},
	}
}

// AuthMiddleware returns the middleware that wraps every webchat HTTP handler.
// It authenticates the user (delegating to knot's ApiAuth + permission check)
// and injects the per-user MCP tool provider into the request context so that
// StandardHost.ListTools / CallTool resolve the user's script and method tools.
func AuthMiddleware(apiAuthMiddleware func(http.Handler) http.Handler, mcpServer *mcplib.Server, scriptToolsProvider ScriptToolsProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// First authenticate (sets user in context), then inject MCP tools.
		withTools := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			ctx = context.WithValue(ctx, "mcp", mcpServer)
			if scriptToolsProvider != nil {
				if user, ok := ctx.Value("user").(*model.User); ok && user != nil {
					if provider := scriptToolsProvider(ctx, user); provider != nil {
						ctx = mcplib.WithToolProviders(ctx, provider)
					}
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
		return apiAuthMiddleware(withTools)
	}
}

// PersonaSource returns a webchat.PersonaSource backed by knot's single
// system-defined persona (loaded from the configured system prompt file).
func PersonaSource() webchat.PersonaSource {
	cfg := config.GetServerConfig()
	systemPrompt := ""
	if cfg != nil {
		systemPrompt = cfg.Chat.SystemPrompt
	}
	return webchat.StaticPersonas{{
		ID:           "default",
		Name:         "Default",
		SystemPrompt: systemPrompt,
		DefaultModel: defaultModelName(),
	}}
}

// defaultModelName returns the configured chat model, or empty if not set.
func defaultModelName() string {
	cfg := config.GetServerConfig()
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Chat.Model)
}

// CommandSource returns a webchat.CommandSource that resolves per-user slash
// commands from the knot database.
type commandSource struct{}

func NewCommandSource() webchat.CommandSource {
	return &commandSource{}
}

func (s *commandSource) Commands(ctx context.Context) ([]webchat.SlashCommand, error) {
	user, _ := ctx.Value("user").(*model.User)
	if user == nil {
		return nil, nil
	}

	cmdService := service.GetCommandService()
	global, _ := cmdService.ListCommands(service.CommandListOptions{FilterUserId: "", User: user})
	own, _ := cmdService.ListCommands(service.CommandListOptions{FilterUserId: user.Id, User: user})

	out := make([]webchat.SlashCommand, 0, len(global)+len(own))
	seen := map[string]bool{}
	for _, c := range append(global, own...) {
		if seen[c.Id] || !c.Active || c.IsDeleted {
			continue
		}
		seen[c.Id] = true
		out = append(out, webchat.SlashCommand{
			ID:           c.Id,
			Name:         c.Name,
			Description:  c.Description,
			ArgumentHint: c.ArgumentHint,
			AllowedTools: strings.Join(c.AllowedTools, ","),
			Body:         c.Body,
			Source:       "knot",
		})
	}
	return out, nil
}

// userFromCtx extracts the authenticated user from the request context.
func userFromCtx(ctx context.Context) *model.User {
	user, _ := ctx.Value("user").(*model.User)
	return user
}

// eventBroadcaster holds the webchat SSE broadcaster so the API
// layer can push notifications (commands_changed etc.) to connected
// chat clients without a direct dependency on the webchat Server.
var eventBroadcaster *webchat.EventBroadcaster

// SetEventBroadcaster stores the broadcaster for later use by
// BroadcastCommandEvent. Called once during server startup.
func SetEventBroadcaster(b *webchat.EventBroadcaster) {
	eventBroadcaster = b
}

// BroadcastCommandEvent pushes a commands_changed event to all
// connected chat clients so they reload their slash command list.
func BroadcastCommandEvent() {
	if eventBroadcaster != nil {
		eventBroadcaster.Broadcast(webchat.ServerEvent{Type: "commands_changed"})
	}
}
