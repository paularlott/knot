package chat

import "github.com/paularlott/knot/internal/openai"

type ChatMessage struct {
	Role       string            `json:"role"` // "user", "assistant", "system", "tool"
	Content    string            `json:"content"`
	Timestamp  int64             `json:"timestamp"`
	ToolCalls  []openai.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

// ChatCompletionResponse represents the response from non-streaming chat completion
type ChatCompletionResponse struct {
	Content string `json:"content"`
}
