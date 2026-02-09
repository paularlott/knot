package chat

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
	mcpopenai "github.com/paularlott/mcp/ai/openai"
)

func createTestConfig() config.ChatConfig {
	return config.ChatConfig{
		Enabled:         true,
		OpenAIAPIKey:    "test-api-key",
		OpenAIBaseURL:   "https://api.openai.com/v1/",
		Model:           "gpt-4",
		MaxTokens:       1000,
		Temperature:     0.7,
		ReasoningEffort: "medium",
		SystemPrompt:    "You are a helpful assistant.",
	}
}

func TestService_ConvertMessages(t *testing.T) {
	config := createTestConfig()
	service := &Service{
		config: config,
	}

	tests := []struct {
		name     string
		messages []ChatMessage
		expected int // expected number of OpenAI messages (including system prompt)
	}{
		{
			name:     "empty messages",
			messages: []ChatMessage{},
			expected: 1, // just system prompt
		},
		{
			name: "single user message",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			expected: 2, // system + user
		},
		{
			name: "conversation with tool calls",
			messages: []ChatMessage{
				{Role: "user", Content: "What's the weather?"},
				{Role: "assistant", Content: "I'll check the weather for you.", ToolCalls: []mcpopenai.ToolCall{
					{ID: "call_1", Type: "function", Function: mcpopenai.ToolCallFunction{Name: "get_weather"}},
				}},
				{Role: "tool", Content: "Sunny, 25°C", ToolCallID: "call_1"},
			},
			expected: 4, // system + user + assistant + tool
		},
		{
			name: "skip system messages from history",
			messages: []ChatMessage{
				{Role: "system", Content: "Old system prompt"},
				{Role: "user", Content: "Hello"},
			},
			expected: 2, // new system + user (old system skipped)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.convertMessagesToOpenAI(tt.messages)
			if len(result) != tt.expected {
				t.Errorf("convertMessages() returned %d messages, expected %d", len(result), tt.expected)
			}

			// First message should always be system
			if len(result) > 0 && result[0].Role != "system" {
				t.Errorf("First message should be system, got %s", result[0].Role)
			}
		})
	}
}
