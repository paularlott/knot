package openai

import (
	"encoding/json"
)

// ModelsResponse represents the response from the models endpoint
type ModelsResponse struct {
	Data   []Model `json:"data"`
	Object string  `json:"object"`
}

// Model represents a single model from the OpenAI API
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ChatCompletionRequest represents a request to the chat completions endpoint
type ChatCompletionRequest struct {
	Model           string    `json:"model"`
	Messages        []Message `json:"messages"`
	Tools           []Tool    `json:"tools,omitempty"`
	MaxTokens       int       `json:"max_tokens,omitempty"`
	Temperature     float32   `json:"temperature,omitempty"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	Stream          bool      `json:"stream"`
}

// ChatCompletionResponse represents a response from the chat completions endpoint
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage,omitempty"`
}

// Choice represents a single choice in the response
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message,omitempty"`
	Delta        Delta   `json:"delta,omitempty"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

// Message represents a message in the conversation
type Message struct {
	Role       string     `json:"role,omitempty"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Delta represents incremental changes in streaming responses
type Delta struct {
	Role             string          `json:"role,omitempty"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content"`
	ToolCalls        []DeltaToolCall `json:"tool_calls,omitempty"`
}

// DeltaToolCall represents incremental tool call data in streaming
type DeltaToolCall struct {
	Index    int           `json:"index"`
	ID       string        `json:"id,omitempty"`
	Type     string        `json:"type,omitempty"`
	Function DeltaFunction `json:"function,omitempty"`
}

// DeltaFunction represents incremental function call data
type DeltaFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ToolCall represents a function call request
type ToolCall struct {
	Index    int              `json:"index,omitempty"`
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction represents the function part of a tool call
type ToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"-"` // Custom marshaling handles this
}

// Tool represents a tool definition
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction represents a function definition
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Custom JSON marshaling for ToolCallFunction
func (tcf ToolCallFunction) MarshalJSON() ([]byte, error) {
	var argsJSON string

	if tcf.Arguments == nil {
		argsJSON = "{}"
	} else {
		// Convert arguments map to JSON string
		argsBytes, err := json.Marshal(tcf.Arguments)
		if err != nil {
			argsJSON = "{}"
		} else {
			argsJSON = string(argsBytes)
		}
	}

	// Create a temporary struct for marshaling
	temp := struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{
		Name:      tcf.Name,
		Arguments: argsJSON,
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
		tcf.Arguments = make(map[string]any)
	} else {
		if err := json.Unmarshal([]byte(temp.Arguments), &tcf.Arguments); err != nil {
			tcf.Arguments = make(map[string]any)
		}
	}

	return nil
}
