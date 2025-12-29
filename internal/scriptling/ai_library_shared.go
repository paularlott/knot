package scriptling

// ChatMessage represents a chat message
type ChatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Messages []ChatMessage `json:"messages"`
}

// ChatCompletionResponse represents a chat completion response
type ChatCompletionResponse struct {
	Content string `json:"content"`
}

// Tool represents a tool and its parameters
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCallRequest represents a tool call request
type ToolCallRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolCallResponse represents a tool call response
type ToolCallResponse struct {
	Content interface{} `json:"content"`
}
