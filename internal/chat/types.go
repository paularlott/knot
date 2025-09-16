package chat

import "github.com/paularlott/knot/internal/openai"

type ChatMessage struct {
	Role       string            `json:"role"` // "user", "assistant", "system", "tool"
	Content    string            `json:"content"`
	Timestamp  int64             `json:"timestamp"`
	ToolCalls  []openai.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
}

type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

type SSEEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}
