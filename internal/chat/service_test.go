package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/openai"
)

// Test data structures
func createTestUser() *model.User {
	return &model.User{
		Id:       "test-user-123",
		Username: "testuser",
	}
}

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

// Mock HTTP server for OpenAI API
func createMockOpenAIServer(t *testing.T, responses []string) *httptest.Server {
	responseIndex := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			if responseIndex < len(responses) {
				w.Write([]byte(responses[responseIndex]))
				responseIndex++
			}
		} else if r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data": [{"id": "gpt-4", "object": "model"}]}`))
		}
	}))
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
				{Role: "assistant", Content: "I'll check the weather for you.", ToolCalls: []openai.ToolCall{
					{ID: "call_1", Type: "function", Function: openai.ToolCallFunction{Name: "get_weather"}},
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

func TestService_FormatUserFriendlyError(t *testing.T) {
	service := &Service{}

	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{
			name:     "nil error",
			err:      nil,
			contains: "unknown error",
		},
		{
			name:     "timeout error",
			err:      context.DeadlineExceeded,
			contains: "taking too long",
		},
		{
			name:     "connection error",
			err:      &mockError{msg: "connection refused"},
			contains: "Unable to connect",
		},
		{
			name:     "rate limit error",
			err:      &mockError{msg: "rate limit exceeded"},
			contains: "currently busy",
		},
		{
			name:     "auth error",
			err:      &mockError{msg: "401 unauthorized"},
			contains: "authentication failed",
		},
		{
			name:     "MCP error",
			err:      &mockError{msg: "MCP tool failed"},
			contains: "MCP tool failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.formatUserFriendlyError(tt.err)
			if !strings.Contains(strings.ToLower(result), strings.ToLower(tt.contains)) {
				t.Errorf("formatUserFriendlyError() = %q, should contain %q", result, tt.contains)
			}
		})
	}
}

// Mock error type for testing
type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}

// Integration test placeholder - would need more setup for full testing
func TestService_Integration(t *testing.T) {
	t.Skip("Integration test - requires full setup")

	// This would test the full flow:
	// 1. Create service with mock OpenAI server
	// 2. Send chat request
	// 3. Verify streaming response
	// 4. Test tool calls if MCP server available
}
