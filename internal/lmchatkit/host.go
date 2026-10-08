// Package lmchatkit adapts the github.com/paularlott/lmchatkit library to knot's
// per-user, MCP-backed chat architecture.
//
// Knot uses lmchatkit.StandardHost (same as llmrouter) with the OpenAI base URL
// pointing at knot's configured LLM endpoint. This is required because the
// MCP AI client's StreamChatCompletion SUPPRESSES tool-call delta chunks when
// MCP servers are present — lmchatkit's manual approval flow needs those chunks
// to reach the frontend. StandardHost uses TranslateOpenAIStream on the raw
// HTTP response, bypassing the suppression.
//
// Per-user MCP tools are injected via the auth middleware (which wraps every
// lmchatkit route): it sets up the tool provider in the request context so
// StandardHost.ListTools / CallTool resolve the user's tools through the MCP
// server's context-aware methods.
package lmchatkit

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	internalmcp "github.com/paularlott/knot/internal/mcp"
	"github.com/paularlott/knot/internal/service"

	"github.com/paularlott/lmchatkit"
	mcplib "github.com/paularlott/mcp"
	"github.com/paularlott/mcp/ai"
)

// ScriptToolsProvider returns the per-user MCP tool providers (script tools,
// method tools, remote servers), matching the MCPServerContext middleware
// logic. Returned as a slice — not merged into one mcp.NewMultiProvider —
// so each provider stays individually visible to mcp.GetToolProviders(ctx):
// StandardHost's SourceScopedHost support needs to type-assert the remote
// server provider as a lmchatkit.SourcedToolProvider, which a MultiProvider
// wrapper would hide (it only forwards GetTools/ExecuteTool).
type ScriptToolsProvider func(ctx context.Context, user *model.User) []mcplib.ToolProvider

// NewHost builds a lmchatkit.StandardHost configured for knot's LLM endpoint,
// MCP server, and single persona. The per-user tool provider is injected by
// AuthMiddleware (callers must wrap lmchatkit's routes with it).
func NewHost(cfg config.ChatConfig, mcpServer *mcplib.Server, scriptToolsProvider ScriptToolsProvider) *lmchatkit.StandardHost {
	return &lmchatkit.StandardHost{
		ModelsFunc: func(ctx context.Context) ([]lmchatkit.Model, error) {
			if cfg.Model == "" {
				return nil, nil
			}
			return []lmchatkit.Model{{ID: cfg.Model}}, nil
		},
		ChatCompletionsURL: ChatCompletionsURL(cfg.Provider, cfg.BaseURL),
		OpenAIToken:        cfg.APIKey,
		MCPServer: func(ctx context.Context) *mcplib.Server {
			return mcpServer
		},
		SystemPromptAugmenter: func(ctx context.Context, current string) string {
			user := userFromCtx(ctx)
			if user == nil {
				return current
			}
			return current + internalmcp.BuildSkillsPrompt(ctx, user,
				"Call the lmchatkit__get_skill tool with the skill URI to retrieve detailed instructions:")
		},
	}
}

// ChatCompletionsURL returns the OpenAI-compatible chat completions URL for a
// provider's base URL. The web chat always speaks the OpenAI protocol, which
// Gemini serves under /openai. A base URL without a path (e.g.
// http://host:1234) gets /v1, as before.
func ChatCompletionsURL(provider, baseURL string) string {
	base := strings.TrimSuffix(baseURL, "/")
	if provider == string(ai.ProviderGemini) {
		return base + "/openai/chat/completions"
	}
	if u, err := url.Parse(base); err == nil && (u.Path == "" || u.Path == "/") {
		return base + "/v1/chat/completions"
	}
	return base + "/chat/completions"
}

// AuthMiddleware returns the middleware that wraps every lmchatkit HTTP handler.
// It authenticates the user (delegating to knot's ApiAuth + permission check)
// and injects the per-user MCP tool, skill and resource providers into the
// request context so that StandardHost.ListTools/CallTool resolve the user's
// script and method tools, StandardHost surfaces the skills extension
// (including the lmchatkit__get_skill virtual tool and its reads), and
// StandardHost.ReadResource can fetch an MCP Apps view's linked ui://
// resource from whichever of the user's remote servers registered it.
func AuthMiddleware(apiAuthMiddleware func(http.Handler) http.Handler, mcpServer *mcplib.Server, scriptToolsProvider ScriptToolsProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// First authenticate (sets user in context), then inject MCP tools.
		withTools := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			ctx = context.WithValue(ctx, "mcp", mcpServer)
			if user, ok := ctx.Value("user").(*model.User); ok && user != nil {
				if scriptToolsProvider != nil {
					if providers := scriptToolsProvider(ctx, user); len(providers) > 0 {
						ctx = mcplib.WithToolProviders(ctx, providers...)
					}
				}
				// knot's own skills first (so a local skill:// URI wins any
				// collision), then remote servers' resources (their tools are
				// already covered by scriptToolsProvider above) — a separate
				// provider instance, but backed by the same cached clients.
				skills := internalmcp.NewSkillsProvider(user)
				ctx = mcplib.WithSkillProviders(ctx, skills)
				ctx = mcplib.WithResourceProviders(ctx, skills, internalmcp.NewRemoteServerProvider(user))
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
		return apiAuthMiddleware(withTools)
	}
}

// PersonaSource returns a lmchatkit.PersonaSource backed by knot's single
// system-defined persona (loaded from the configured system prompt file).
func PersonaSource() lmchatkit.PersonaSource {
	cfg := config.GetServerConfig()
	systemPrompt := ""
	if cfg != nil {
		systemPrompt = cfg.Chat.SystemPrompt
	}
	return lmchatkit.StaticPersonas{{
		ID:           "default",
		Name:         "Default",
		SystemPrompt: systemPrompt,
		DefaultModel: defaultModelName(),
		Params:       personaParams(cfg),
	}}
}

// personaParams returns the model params set in the chat config.
func personaParams(cfg *config.ServerConfig) map[string]interface{} {
	params := map[string]interface{}{}
	if cfg != nil && cfg.Chat.ReasoningEffort != "" {
		params["reasoning_effort"] = cfg.Chat.ReasoningEffort
	}
	return params
}

// defaultModelName returns the configured chat model, or empty if not set.
func defaultModelName() string {
	cfg := config.GetServerConfig()
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Chat.Model)
}

// CommandSource returns a lmchatkit.CommandSource that resolves per-user slash
// commands from the knot database.
type commandSource struct{}

func NewCommandSource() lmchatkit.CommandSource {
	return &commandSource{}
}

func (s *commandSource) Commands(ctx context.Context) ([]lmchatkit.SlashCommand, error) {
	user, _ := ctx.Value("user").(*model.User)
	if user == nil {
		return nil, nil
	}

	cmdService := service.GetCommandService()
	global, _ := cmdService.ListCommands(service.CommandListOptions{FilterUserId: "", User: user})
	own, _ := cmdService.ListCommands(service.CommandListOptions{FilterUserId: user.Id, User: user})

	out := make([]lmchatkit.SlashCommand, 0, len(global)+len(own))
	seen := map[string]bool{}
	for _, c := range append(global, own...) {
		if seen[c.Id] || !c.Active || c.IsDeleted {
			continue
		}
		seen[c.Id] = true
		out = append(out, lmchatkit.SlashCommand{
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

// eventBroadcaster holds the lmchatkit SSE broadcaster so the API
// layer can push notifications (commands_changed etc.) to connected
// chat clients without a direct dependency on the lmchatkit Server.
var eventBroadcaster *lmchatkit.EventBroadcaster

// SetEventBroadcaster stores the broadcaster for later use by
// BroadcastCommandEvent. Called once during server startup.
func SetEventBroadcaster(b *lmchatkit.EventBroadcaster) {
	eventBroadcaster = b
}

// BroadcastCommandEvent pushes a commands_changed event to all
// connected chat clients so they reload their slash command list.
func BroadcastCommandEvent() {
	if eventBroadcaster != nil {
		eventBroadcaster.Broadcast(lmchatkit.ServerEvent{Type: "commands_changed"})
	}
}
