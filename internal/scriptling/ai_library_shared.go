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
