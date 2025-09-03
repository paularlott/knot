package openai

// CompletionAccumulator accumulates streaming chunks into a complete ChatCompletionResponse
type CompletionAccumulator struct {
	Choices []Choice `json:"choices"`
}

// AddChunk adds a streaming chunk to the accumulator
func (acc *CompletionAccumulator) AddChunk(chunk ChatCompletionResponse) {
	if len(chunk.Choices) == 0 {
		return
	}

	// Initialize choices if needed
	for len(acc.Choices) <= chunk.Choices[0].Index {
		acc.Choices = append(acc.Choices, Choice{
			Index: len(acc.Choices),
			Message: Message{
				Role: "assistant",
			},
		})
	}

	choice := &acc.Choices[chunk.Choices[0].Index]
	delta := chunk.Choices[0].Delta

	// Accumulate content
	if delta.Content != "" {
		currentContent := choice.Message.GetContentAsString()
		choice.Message.SetContentAsString(currentContent + delta.Content)
	}

	// Accumulate tool calls
	if len(delta.ToolCalls) > 0 {
		for _, deltaToolCall := range delta.ToolCalls {
			// Extend tool calls array if needed
			for len(choice.Message.ToolCalls) <= deltaToolCall.Index {
				choice.Message.ToolCalls = append(choice.Message.ToolCalls, ToolCall{})
			}

			toolCall := &choice.Message.ToolCalls[deltaToolCall.Index]

			if deltaToolCall.ID != "" {
				toolCall.ID = deltaToolCall.ID
			}
			if deltaToolCall.Type != "" {
				toolCall.Type = deltaToolCall.Type
			}
			if deltaToolCall.Function.Name != "" {
				toolCall.Function.Name = deltaToolCall.Function.Name
			}
			if deltaToolCall.Function.Arguments != "" {
				// Initialize arguments map if needed
				if toolCall.Function.Arguments == nil {
					toolCall.Function.Arguments = make(map[string]any)
				}
				// For streaming, we need to accumulate the JSON string and parse at the end
				// This is a simplified approach - in practice you'd need proper JSON streaming
				if existingArgs, ok := toolCall.Function.Arguments["_raw"].(string); ok {
					toolCall.Function.Arguments["_raw"] = existingArgs + deltaToolCall.Function.Arguments
				} else {
					toolCall.Function.Arguments["_raw"] = deltaToolCall.Function.Arguments
				}
			}
		}
	}

	// Set finish reason
	if chunk.Choices[0].FinishReason != "" {
		choice.FinishReason = chunk.Choices[0].FinishReason
	}
}

// FinishedContent returns the content if it was completed
func (acc *CompletionAccumulator) FinishedContent() (string, bool) {
	if len(acc.Choices) == 0 || acc.Choices[0].FinishReason == "" {
		return "", false
	}
	return acc.Choices[0].Message.GetContentAsString(), true
}

// FinishedToolCall returns a tool call if it was completed
func (acc *CompletionAccumulator) FinishedToolCall() (*ToolCall, bool) {
	if len(acc.Choices) == 0 || acc.Choices[0].FinishReason == "" {
		return nil, false
	}

	toolCalls := acc.Choices[0].Message.ToolCalls
	if len(toolCalls) == 0 {
		return nil, false
	}

	// Return the last tool call
	return &toolCalls[len(toolCalls)-1], true
}

// FinishedRefusal returns the refusal if it was completed
func (acc *CompletionAccumulator) FinishedRefusal() (string, bool) {
	if len(acc.Choices) == 0 || acc.Choices[0].FinishReason != "content_filter" {
		return "", false
	}

	// In this implementation, we consider content_filter finish reason as refusal
	return "Content filtered", true
}
