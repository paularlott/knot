package chat

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
)

//go:embed tool-system-prompt.md
var defaultToolSystemPrompt string

const toolCallMarker = "```tool_call"
const toolCallEnd = "```"

type pseudoBuffer struct {
	buffer       strings.Builder
	inToolCall   bool
	toolCallData strings.Builder
	windowSize   int
}

func newPseudoBuffer() *pseudoBuffer {
	return &pseudoBuffer{
		windowSize: len(toolCallMarker) + 10, // Buffer size for marker detection
	}
}

func (s *Service) generateToolSystemPrompt(ctx context.Context, user *model.User) (string, error) {
	if s.mcpServer == nil {
		return "", nil
	}

	tools := s.mcpServer.ListTools()
	if len(tools) == 0 {
		return "", nil
	}

	// Load tool system prompt
	toolSystemPrompt := defaultToolSystemPrompt
	if s.config.SystemPromptFile != "" {
		content, err := os.ReadFile(s.config.SystemPromptFile)
		if err != nil {
			return "", fmt.Errorf("failed to read tool system prompt file %s: %w", s.config.SystemPromptFile, err)
		}
		toolSystemPrompt = string(content)
	}

	var prompt strings.Builder
	prompt.WriteString(toolSystemPrompt)
	prompt.WriteString("\n\n")

	for _, tool := range tools {
		prompt.WriteString(fmt.Sprintf("### %s\n", tool.Name))
		prompt.WriteString(fmt.Sprintf("**Description:** %s\n\n", tool.Description))

		if schema, ok := tool.InputSchema.(map[string]interface{}); ok {
			if props, ok := schema["properties"].(map[string]interface{}); ok {
				prompt.WriteString("**Parameters:**\n")
				for name, prop := range props {
					if propMap, ok := prop.(map[string]interface{}); ok {
						propType := "string"
						if t, ok := propMap["type"].(string); ok {
							propType = t
						}
						desc := ""
						if d, ok := propMap["description"].(string); ok {
							desc = d
						}
						required := false
						if req, ok := schema["required"].([]interface{}); ok {
							for _, r := range req {
								if r.(string) == name {
									required = true
									break
								}
							}
						}
						reqStr := ""
						if required {
							reqStr = " (required)"
						}
						prompt.WriteString(fmt.Sprintf("- `%s` (%s)%s: %s\n", name, propType, reqStr, desc))
					}
				}
				prompt.WriteString("\n")
			}
		}
	}

	return prompt.String(), nil
}

func (s *Service) streamChatPseudo(ctx context.Context, messages []ChatMessage, user *model.User, w http.ResponseWriter, r *http.Request) error {
	if len(messages) == 0 {
		return nil
	}

	sseWriter := rest.NewSSEStreamWriter(w, r)
	defer sseWriter.Close()

	openAIMessages := s.convertMessagesPseudo(ctx, messages, user)

	req := OpenAIRequest{
		Model:           s.config.Model,
		Messages:        openAIMessages,
		MaxTokens:       s.config.MaxTokens,
		Temperature:     s.config.Temperature,
		ReasoningEffort: s.config.ReasoningEffort,
		Stream:          true,
	}

	return s.callOpenAIPseudo(ctx, req, user, sseWriter, openAIMessages)
}

func (s *Service) convertMessagesPseudo(ctx context.Context, messages []ChatMessage, user *model.User) []OpenAIMessage {
	var openAIMessages []OpenAIMessage

	// Add tool system prompt first
	if toolPrompt, err := s.generateToolSystemPrompt(ctx, user); err == nil && toolPrompt != "" {
		openAIMessages = append(openAIMessages, OpenAIMessage{
			Role:    "system",
			Content: toolPrompt,
		})
	}

	// Convert chat messages, skipping system messages from history
	for _, msg := range messages {
		if msg.Role != "system" {
			content := msg.Content
			if content == "" {
				content = " " // Ensure non-empty content
			}
			openAIMessages = append(openAIMessages, OpenAIMessage{
				Role:    msg.Role,
				Content: content,
			})
		}
	}

	return openAIMessages
}

func (s *Service) callOpenAIPseudo(ctx context.Context, req OpenAIRequest, user *model.User, sseWriter *rest.SSEStreamWriter, conversationHistory []OpenAIMessage) error {
	buffer := newPseudoBuffer()

	return rest.StreamData[*OpenAIResponse, OpenAIResponse](
		s.restClient,
		ctx,
		"POST",
		"chat/completions",
		req,
		func(response *OpenAIResponse) (bool, error) {
			return s.processPseudoChunk(ctx, *response, user, sseWriter, conversationHistory, buffer)
		},
		rest.StreamSSE,
	)
}

func (s *Service) processPseudoChunk(ctx context.Context, response OpenAIResponse, user *model.User, sseWriter *rest.SSEStreamWriter, conversationHistory []OpenAIMessage, buffer *pseudoBuffer) (bool, error) {
	if len(response.Choices) == 0 {
		return false, nil
	}

	choice := response.Choices[0]
	content := choice.Delta.Content

	if content != "" {
		buffer.buffer.WriteString(content)
		bufferContent := buffer.buffer.String()

		// Check for JSON-like content that might be a tool call
		if !buffer.inToolCall {
			// Look for tool call marker first
			if idx := strings.Index(bufferContent, toolCallMarker); idx != -1 {
				beforeToolCall := bufferContent[:idx]
				if beforeToolCall != "" {
					sseWriter.WriteChunk(SSEEvent{
						Type: "content",
						Data: beforeToolCall,
					})
				}
				buffer.inToolCall = true
				buffer.toolCallData.Reset()
				buffer.buffer.Reset()
				afterMarker := bufferContent[idx+len(toolCallMarker):]
				if strings.HasPrefix(afterMarker, "\n") {
					afterMarker = afterMarker[1:]
				}
				buffer.toolCallData.WriteString(afterMarker)
			} else {
				// Normal content handling
				if len(bufferContent) > buffer.windowSize {
					toSend := bufferContent[:len(bufferContent)-len(toolCallMarker)]
					if toSend != "" {
						sseWriter.WriteChunk(SSEEvent{
							Type: "content",
							Data: toSend,
						})
					}
					buffer.buffer.Reset()
					buffer.buffer.WriteString(bufferContent[len(bufferContent)-len(toolCallMarker):])
				}
			}
		} else {
			// Inside tool call, add new content and check for end marker
			buffer.toolCallData.WriteString(content)
			toolCallDataStr := buffer.toolCallData.String()
			if idx := strings.Index(toolCallDataStr, toolCallEnd); idx != -1 {
				// Extract just the JSON part (before the end marker)
				toolCallJSON := toolCallDataStr[:idx]
				// Get remaining content before resetting
				remaining := toolCallDataStr[idx+len(toolCallEnd):]

				if err := s.processPseudoToolCall(ctx, toolCallJSON, user, sseWriter, conversationHistory); err != nil {
					return true, err
				}
				buffer.inToolCall = false
				buffer.toolCallData.Reset()
				buffer.buffer.Reset()
				// Handle any remaining content after the end marker
				if remaining != "" {
					buffer.buffer.WriteString(remaining)
				}
			}
		}
	}

	// Handle finish
	if choice.FinishReason != "" && choice.FinishReason != "tool_calls" {
		// Send any remaining buffer content
		if buffer.buffer.Len() > 0 {
			sseWriter.WriteChunk(SSEEvent{
				Type: "content",
				Data: buffer.buffer.String(),
			})
		}

		// If we have incomplete tool call, treat as regular content
		if buffer.inToolCall && buffer.toolCallData.Len() > 0 {
			sseWriter.WriteChunk(SSEEvent{
				Type: "content",
				Data: toolCallMarker + buffer.toolCallData.String(),
			})
		}

		sseWriter.WriteChunk(SSEEvent{
			Type: "done",
			Data: nil,
		})
		return true, nil
	}

	return false, nil
}

func (s *Service) processPseudoToolCall(ctx context.Context, toolCallJSON string, user *model.User, sseWriter *rest.SSEStreamWriter, conversationHistory []OpenAIMessage) error {
	// Parse single tool call JSON
	var toolCall struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := json.Unmarshal([]byte(strings.TrimSpace(toolCallJSON)), &toolCall); err != nil {
		return nil // Skip invalid JSON
	}

	// Create OpenAI-compatible tool call for frontend
	openAIToolCall := ToolCall{
		ID:   fmt.Sprintf("call_%d", len(conversationHistory)),
		Type: "function",
		Function: ToolCallFunction{
			Name:      toolCall.Name,
			Arguments: toolCall.Arguments,
		},
	}

	// Send tool call to frontend
	sseWriter.WriteChunk(SSEEvent{
		Type: "tool_calls",
		Data: []ToolCall{openAIToolCall},
	})

	// Execute tool
	result, err := s.executeMCPTool(ctx, openAIToolCall, user)
	if err != nil {
		result = fmt.Sprintf("Error executing tool: %v", err)
	}

	// Send tool result to frontend
	sseWriter.WriteChunk(SSEEvent{
		Type: "tool_result",
		Data: map[string]interface{}{
			"tool_name":    toolCall.Name,
			"result":       result,
			"tool_call_id": openAIToolCall.ID,
		},
	})

	// Update conversation history
	conversationHistory = append(conversationHistory,
		OpenAIMessage{
			Role:      "assistant",
			Content:   "I'll help you with that. Let me use the available tools.",
			ToolCalls: []ToolCall{openAIToolCall},
		},
		OpenAIMessage{
			Role:       "tool",
			Content:    result,
			ToolCallID: openAIToolCall.ID,
		},
	)

	// Continue conversation with tool results
	req := OpenAIRequest{
		Model:           s.config.Model,
		Messages:        conversationHistory,
		MaxTokens:       s.config.MaxTokens,
		Temperature:     s.config.Temperature,
		ReasoningEffort: s.config.ReasoningEffort,
		Stream:          true,
	}

	return s.callOpenAIPseudo(ctx, req, user, sseWriter, conversationHistory)
}
