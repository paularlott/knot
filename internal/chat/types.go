package chat

import "encoding/json"

type ChatMessage struct {
	Role       string     `json:"role"` // "user", "assistant", "system", "tool"
	Content    string     `json:"content"`
	Timestamp  int64      `json:"timestamp"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
	Index    int              `json:"index"`
}

type ToolCallFunction struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"-"`
}

// Custom JSON marshaling for ToolCallFunction
func (tcf ToolCallFunction) MarshalJSON() ([]byte, error) {
	var argsJSON []byte
	var err error

	if tcf.Arguments == nil {
		argsJSON = []byte("{}")
	} else {
		// Convert arguments map to JSON string
		argsJSON, err = json.Marshal(tcf.Arguments)
		if err != nil {
			argsJSON = []byte("{}")
		}
	}

	// Create a temporary struct for marshaling
	temp := struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{
		Name:      tcf.Name,
		Arguments: string(argsJSON),
	}

	return json.Marshal(temp)
}

// Custom JSON unmarshaling for ToolCallFunction
func (tcf *ToolCallFunction) UnmarshalJSON(data []byte) error {
	// Create a temporary struct for unmarshaling
	temp := struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	tcf.Name = temp.Name

	// Parse arguments string back to map
	if temp.Arguments == "" || temp.Arguments == "null" {
		tcf.Arguments = make(map[string]interface{})
	} else {
		if err := json.Unmarshal([]byte(temp.Arguments), &tcf.Arguments); err != nil {
			tcf.Arguments = make(map[string]interface{})
		}
	}

	return nil
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
	Index    int                 `json:"index"`
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function OpenAIDeltaFunction `json:"function"`
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
	Type     string             `json:"type"`
	Function OpenAIToolFunction `json:"function"`
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
