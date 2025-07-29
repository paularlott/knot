package chat

type ChatMessage struct {
	Role      string `json:"role"`      // "user", "assistant", "system"
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatConfig struct {
	OpenAIAPIKey     string `json:"openai_api_key"`
	OpenAIBaseURL    string `json:"openai_base_url"`
	Model            string `json:"model"`
	MaxTokens        int    `json:"max_tokens"`
	Temperature      float32 `json:"temperature"`
	SystemPrompt     string `json:"system_prompt"`
}

type ToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function ToolCallFunction       `json:"function"`
	Index    int                    `json:"index"`
}

type ToolCallFunction struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

type SSEEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type OpenAIMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type OpenAIDeltaToolCall struct {
	Index    int                    `json:"index"`
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function OpenAIDeltaFunction    `json:"function"`
}

type OpenAIDeltaFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIDelta struct {
	Content   string                `json:"content"`
	ToolCalls []OpenAIDeltaToolCall `json:"tool_calls"`
}

type OpenAITool struct {
	Type     string                 `json:"type"`
	Function OpenAIToolFunction     `json:"function"`
}

type OpenAIToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float32         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream"`
}

type OpenAIResponse struct {
	Choices []OpenAIChoice `json:"choices"`
}

type OpenAIChoice struct {
	Delta        OpenAIDelta   `json:"delta"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type HTTPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	URL         string                 `json:"url"`
	Method      string                 `json:"method"`
	Headers     map[string]string      `json:"headers"`
	Parameters  map[string]interface{} `json:"parameters"`
}