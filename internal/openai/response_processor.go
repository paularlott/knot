package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
)

// responseProcessor handles the conversion and processing of Responses API requests
type responseProcessor struct {
	client *Client
}

// Process processes a response request and returns the result
func (p *responseProcessor) Process(ctx context.Context, response *model.Response) (map[string]interface{}, error) {
	// Extract the request
	var req CreateResponseRequest
	if err := response.GetRequest(&req); err != nil {
		return nil, fmt.Errorf("failed to extract request: %w", err)
	}

	// Convert input to chat completion messages
	messages, err := p.convertInputToMessages(req.Input)
	if err != nil {
		return nil, fmt.Errorf("failed to convert input: %w", err)
	}

	// Add instructions if provided
	if req.Instructions != "" {
		systemMsg := Message{Role: "system"}
		systemMsg.SetContentAsString(req.Instructions)
		messages = append([]Message{systemMsg}, messages...)
	}

	// Handle previous_response_id for conversation chaining
	if req.PreviousResponseID != "" {
		db := database.GetInstance()
		prevResponse, err := db.GetResponse(req.PreviousResponseID)
		if err != nil {
			return nil, fmt.Errorf("failed to load previous response: %w", err)
		}
		if prevResponse == nil || prevResponse.IsDeleted {
			return nil, fmt.Errorf("previous response not found")
		}
		if prevResponse.Status != model.StatusCompleted {
			return nil, fmt.Errorf("previous response is not completed (status: %s)", prevResponse.Status)
		}

		// Extract output from previous response
		var prevRespData map[string]interface{}
		if err := prevResponse.GetResponse(&prevRespData); err != nil {
			return nil, fmt.Errorf("failed to extract previous response data: %w", err)
		}

		// Convert previous output to messages and append
		prevMessages, err := p.convertOutputToMessages(prevRespData)
		if err != nil {
			return nil, fmt.Errorf("failed to convert previous output to messages: %w", err)
		}

		// Append previous messages before current messages
		messages = append(prevMessages, messages...)
		log.Debug("Appended previous response messages", "previous_id", req.PreviousResponseID, "message_count", len(prevMessages))
	}

	// Create chat completion request
	chatReq := ChatCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		MaxTokens:   1000, // Default token limit
		Temperature: 0.7, // Default temperature
	}

	// Call ChatCompletion - this handles tool calls internally
	chatResp, err := p.client.ChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	// Convert chat completion response to Responses API format
	result := p.convertChatCompletionToResponse(chatResp, req)
	return result, nil
}

// convertInputToMessages converts various input formats to Messages
func (p *responseProcessor) convertInputToMessages(input []any) ([]Message, error) {
	var messages []Message

	for _, item := range input {
		switch v := item.(type) {
		case string:
			// Simple string input - convert to user message
			msg := Message{Role: "user"}
			msg.SetContentAsString(v)
			messages = append(messages, msg)

		case map[string]interface{}:
			// Map input - parse as message
			msgBytes, err := json.Marshal(item)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal input item: %w", err)
			}
			var msg Message
			if err := json.Unmarshal(msgBytes, &msg); err != nil {
				return nil, fmt.Errorf("failed to unmarshal message: %w", err)
			}
			messages = append(messages, msg)

		case []interface{}:
			// Nested array - recursively convert
			nestedMessages, err := p.convertInputToMessages(v)
			if err != nil {
				return nil, err
			}
			messages = append(messages, nestedMessages...)

		default:
			return nil, fmt.Errorf("unsupported input type: %T", item)
		}
	}

	return messages, nil
}

// convertOutputToMessages converts Responses API output format back to chat completion Messages
func (p *responseProcessor) convertOutputToMessages(responseData map[string]interface{}) ([]Message, error) {
	outputRaw, ok := responseData["output"]
	if !ok {
		return nil, fmt.Errorf("output not found in response data")
	}

	output, ok := outputRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("output is not an array")
	}

	var messages []Message
	for _, item := range output {
		msgMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		role, _ := msgMap["role"].(string)
		contentRaw, hasContent := msgMap["content"]

		msg := Message{Role: role}

		if hasContent {
			contentArray, ok := contentRaw.([]interface{})
			if !ok {
				continue
			}

			// Process content array (may contain output_text, tool_call, etc.)
			var textParts []string
			for _, contentItem := range contentArray {
				contentPart, ok := contentItem.(map[string]interface{})
				if !ok {
					continue
				}

				contentType, _ := contentPart["type"].(string)

				switch contentType {
				case "output_text":
					if text, ok := contentPart["text"].(string); ok {
						textParts = append(textParts, text)
					}
				case "tool_call":
					// For tool calls, we'd need to reconstruct the tool call
					// For now, skip as conversation chaining typically only needs the text
				}
			}

			// Set content as concatenated text
			if len(textParts) > 0 {
				msg.SetContentAsString(textParts[0]) // Use first part for simplicity
			}
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// convertChatCompletionToResponse converts a ChatCompletionResponse to Responses API format
func (p *responseProcessor) convertChatCompletionToResponse(chatResp *ChatCompletionResponse, req CreateResponseRequest) map[string]interface{} {
	result := make(map[string]interface{})

	// Convert output messages to Responses API format
	var output []interface{}
	for _, choice := range chatResp.Choices {
		content := choice.Message.GetContentAsString()
		if content != "" {
			// Text output
			output = append(output, map[string]interface{}{
				"type":  "message",
				"id":     generateMessageID(),
				"status": "completed",
				"role":   choice.Message.Role,
				"content": []map[string]interface{}{
					{
						"type": "output_text",
						"text": content,
					},
				},
			})
		}

		// Handle tool calls
		for _, toolCall := range choice.Message.ToolCalls {
			output = append(output, map[string]interface{}{
				"type":  "message",
				"id":     generateMessageID(),
				"status": "completed",
				"role":   "assistant",
				"content": []map[string]interface{}{
					{
						"type": "tool_call",
						"id":   toolCall.ID,
						"name": toolCall.Function.Name,
						"args": toolCall.Function.Arguments,
					},
				},
			})
		}
	}

	result["output"] = output
	result["usage"] = chatResp.Usage

	// Copy metadata from request if not set
	if req.Metadata != nil {
		result["metadata"] = req.Metadata
	} else {
		result["metadata"] = make(map[string]interface{})
	}

	return result
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	// Simple ID generation - could be UUID or similar
	return fmt.Sprintf("msg_%d", getCurrentTimestamp())
}

// getCurrentTimestamp returns a Unix timestamp
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}
