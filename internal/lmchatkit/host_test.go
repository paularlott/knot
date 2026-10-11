package lmchatkit

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
)

func TestChatCompletionsURL(t *testing.T) {
	for _, tc := range []struct{ provider, base, want string }{
		{"grok", "https://api.x.ai/v1", "https://api.x.ai/v1/chat/completions"},
		{"claude", "https://api.anthropic.com/v1", "https://api.anthropic.com/v1/chat/completions"},
		{"gemini", "https://generativelanguage.googleapis.com/v1beta", "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"},
		{"zai", "https://api.z.ai/api/paas/v4", "https://api.z.ai/api/paas/v4/chat/completions"},
		{"openai", "http://localhost:8085/v1/", "http://localhost:8085/v1/chat/completions"},
		{"openai", "http://localhost:1234", "http://localhost:1234/v1/chat/completions"},
		{"ollama", "http://127.0.0.1:11434/v1", "http://127.0.0.1:11434/v1/chat/completions"},
	} {
		if got := ChatCompletionsURL(tc.provider, tc.base); got != tc.want {
			t.Errorf("ChatCompletionsURL(%q, %q) = %q, want %q", tc.provider, tc.base, got, tc.want)
		}
	}
}

func TestPersonaParams(t *testing.T) {
	if got := personaParams(&config.ServerConfig{Chat: config.ChatConfig{ReasoningEffort: "low"}}); got["reasoning_effort"] != "low" {
		t.Errorf("params = %v, want reasoning_effort low", got)
	}
	if got := personaParams(&config.ServerConfig{}); len(got) != 0 {
		t.Errorf("params = %v, want none", got)
	}
	if got := personaParams(nil); len(got) != 0 {
		t.Errorf("nil config params = %v", got)
	}
}
