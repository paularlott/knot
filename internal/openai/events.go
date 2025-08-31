package openai

// StreamEvent represents a streaming event from the OpenAI client
type StreamEvent interface {
	EventType() string
}

// ContentEvent represents content being streamed
type ContentEvent struct {
	Content string
}

func (e ContentEvent) EventType() string { return "content" }

// ReasoningEvent represents reasoning content (thinking blocks)
type ReasoningEvent struct {
	Content string
}

func (e ReasoningEvent) EventType() string { return "reasoning" }

// ToolCallsEvent represents tool calls being made
type ToolCallsEvent struct {
	ToolCalls []ToolCall
}

func (e ToolCallsEvent) EventType() string { return "tool_calls" }

// ToolResultEvent represents the result of a tool call
type ToolResultEvent struct {
	ToolName   string
	Result     string
	ToolCallID string
}

func (e ToolResultEvent) EventType() string { return "tool_result" }

// ErrorEvent represents an error during streaming
type ErrorEvent struct {
	Error string
}

func (e ErrorEvent) EventType() string { return "error" }

// DoneEvent represents completion of streaming
type DoneEvent struct{}

func (e DoneEvent) EventType() string { return "done" }
