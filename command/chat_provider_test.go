package command

import (
	"testing"

	"github.com/paularlott/mcp/ai"
)

func TestChatProviderForType(t *testing.T) {
	for chatType, want := range map[string]ai.Provider{
		"":          ai.ProviderOpenAI,
		"openai":    ai.ProviderOpenAI,
		"anthropic": ai.ProviderClaude,
		"google":    ai.ProviderGemini,
		"ollama":    ai.ProviderOllama,
		"xai":       ai.ProviderGrok,
		"grok":      ai.ProviderGrok,
		"unknown":   ai.ProviderOpenAI,
	} {
		if got := chatProviderForType(chatType); got != string(want) {
			t.Errorf("chatProviderForType(%q) = %q, want %q", chatType, got, want)
		}
	}
}

func TestResolveChatEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name, chatType, provider, baseURL string
		wantProvider, wantBaseURL         string
	}{
		{"provider alone uses its default URL", "", "grok", "", "grok", "https://api.x.ai/v1"},
		{"provider with base_url override", "", "grok", "https://proxy.example/v1", "grok", "https://proxy.example/v1"},
		{"claude provider", "", "claude", "", "claude", "https://api.anthropic.com/v1"},
		{"gemini provider", "", "gemini", "", "gemini", "https://generativelanguage.googleapis.com/v1beta"},
		{"openai provider", "", "openai", "", "openai", "https://api.openai.com/v1"},
		{"mistral provider", "", "mistral", "", "mistral", "https://api.mistral.ai/v1"},
		{"zai provider", "", "zai", "", "zai", "https://api.z.ai/api/paas/v4"},
		{"ollama provider", "", "ollama", "", "ollama", "http://127.0.0.1:11434/v1"},
		{"type with base_url", "openai", "", "http://localhost:8085/v1/", "openai", "http://localhost:8085/v1/"},
		{"type wins over provider", "openai", "grok", "http://localhost:1234/v1", "openai", "http://localhost:1234/v1"},
		{"type openai without base_url keeps the local default", "openai", "", "", "openai", "http://127.0.0.1:11434/v1"},
		{"type xai without base_url", "xai", "", "", "grok", "https://api.x.ai/v1"},
		{"neither keeps the local default", "", "", "", "openai", "http://127.0.0.1:11434/v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, baseURL := resolveChatEndpoint(tc.chatType, tc.provider, tc.baseURL)
			if provider != tc.wantProvider || baseURL != tc.wantBaseURL {
				t.Errorf("got (%q, %q), want (%q, %q)", provider, baseURL, tc.wantProvider, tc.wantBaseURL)
			}
		})
	}
}

// Every provider has a default base URL, so provider alone is enough.
func TestChatProviderBaseURLsCoverEveryProvider(t *testing.T) {
	for _, p := range []ai.Provider{ai.ProviderOpenAI, ai.ProviderClaude, ai.ProviderGemini, ai.ProviderOllama, ai.ProviderMistral, ai.ProviderZAi, ai.ProviderGrok} {
		if chatProviderBaseURLs[string(p)] == "" {
			t.Errorf("no default base URL for %s", p)
		}
	}
}
