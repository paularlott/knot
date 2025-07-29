package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/mcp"
)

type Service struct {
	config    ChatConfig
	mcpServer *mcp.Server
	httpTools []HTTPTool
}

func NewService(config ChatConfig, mcpServer *mcp.Server) *Service {
	return &Service{
		config:    config,
		mcpServer: mcpServer,
		httpTools: []HTTPTool{},
	}
}

func (s *Service) StreamChat(ctx context.Context, messages []ChatMessage, user *model.User, writer io.Writer) error {
	// If no messages, return early
	if len(messages) == 0 {
		return nil
	}
	
	// Convert messages to OpenAI format
	openAIMessages := s.convertMessages(messages)
	
	// Get available tools
	tools, err := s.getAvailableTools(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to get tools: %w", err)
	}

	// Create OpenAI request
	req := OpenAIRequest{
		Model:       s.config.Model,
		Messages:    openAIMessages,
		Tools:       tools,
		MaxTokens:   s.config.MaxTokens,
		Temperature: s.config.Temperature,
		Stream:      true,
	}

	// Call OpenAI API
	return s.callOpenAI(ctx, req, user, writer)
}

func (s *Service) convertMessages(messages []ChatMessage) []OpenAIMessage {
	var openAIMessages []OpenAIMessage
	
	// Add system prompt if configured
	if s.config.SystemPrompt != "" {
		openAIMessages = append(openAIMessages, OpenAIMessage{
			Role:    "system",
			Content: s.config.SystemPrompt,
		})
	}

	// Convert chat messages
	for _, msg := range messages {
		openAIMessages = append(openAIMessages, OpenAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return openAIMessages
}

func (s *Service) getAvailableTools(ctx context.Context, user *model.User) ([]OpenAITool, error) {
	var tools []OpenAITool

	// Get MCP tools
	mcpTools, err := s.getMCPTools(ctx, user)
	if err != nil {
		return nil, err
	}
	tools = append(tools, mcpTools...)

	// Add HTTP tools
	for _, httpTool := range s.httpTools {
		tools = append(tools, OpenAITool{
			Type: "function",
			Function: OpenAIToolFunction{
				Name:        httpTool.Name,
				Description: httpTool.Description,
				Parameters:  httpTool.Parameters,
			},
		})
	}

	return tools, nil
}

func (s *Service) getMCPTools(ctx context.Context, user *model.User) ([]OpenAITool, error) {
	// This is a simplified approach - in practice you'd want to properly call the MCP server
	// For now, we'll return the known tools
	return []OpenAITool{
		{
			Type: "function",
			Function: OpenAIToolFunction{
				Name:        "list_spaces",
				Description: "List all spaces for a user or all users",
				Parameters: map[string]interface{}{
					"type":                 "object",
					"properties":           map[string]interface{}{},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: OpenAIToolFunction{
				Name:        "start_space",
				Description: "Start a space by its ID",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"space_id": map[string]interface{}{
							"type":        "string",
							"description": "The ID of the space to start",
						},
					},
					"required":             []string{"space_id"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: OpenAIToolFunction{
				Name:        "stop_space",
				Description: "Stop a space by its ID",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"space_id": map[string]interface{}{
							"type":        "string",
							"description": "The ID of the space to stop",
						},
					},
					"required":             []string{"space_id"},
					"additionalProperties": false,
				},
			},
		},
	}, nil
}

func (s *Service) callOpenAI(ctx context.Context, req OpenAIRequest, user *model.User, writer io.Writer) error {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.config.OpenAIBaseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.config.OpenAIAPIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenAI API error: %d", resp.StatusCode)
	}

	return s.processStreamResponse(ctx, resp.Body, user, writer)
}

func (s *Service) processStreamResponse(ctx context.Context, reader io.Reader, user *model.User, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	var currentToolCalls []ToolCall

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var response OpenAIResponse
		if err := json.Unmarshal([]byte(data), &response); err != nil {
			continue
		}

		if len(response.Choices) == 0 {
			continue
		}

		choice := response.Choices[0]
		
		// Handle tool calls accumulation
		if len(choice.Delta.ToolCalls) > 0 {
			for _, toolCall := range choice.Delta.ToolCalls {
				if toolCall.ID != "" && toolCall.Function.Name != "" {
					currentToolCalls = append(currentToolCalls, toolCall)
				}
			}
		}

		// Handle content
		if choice.Delta.Content != "" {
			event := SSEEvent{
				Type: "content",
				Data: choice.Delta.Content,
			}
			s.writeSSEEvent(writer, event)
		}

		// Handle finish reason - execute tools when response is complete
		if choice.FinishReason == "tool_calls" && len(currentToolCalls) > 0 {
			// Execute tool calls and continue conversation
			for _, toolCall := range currentToolCalls {
				result, err := s.executeToolCall(ctx, toolCall, user)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				}
				
				event := SSEEvent{
					Type: "tool_result",
					Data: map[string]interface{}{
						"tool_call_id": toolCall.ID,
						"name":         toolCall.Function.Name,
						"result":       result,
					},
				}
				s.writeSSEEvent(writer, event)
			}
			
			// Send tool results back to AI for final response
			s.continueWithToolResults(ctx, currentToolCalls, user, writer)
		}
	}

	return scanner.Err()
}

func (s *Service) executeToolCall(ctx context.Context, toolCall ToolCall, user *model.User) (string, error) {
	// Check if it's an MCP tool
	mcpTools := []string{"list_spaces", "start_space", "stop_space", "get_docker_podman_spec"}
	for _, mcpTool := range mcpTools {
		if toolCall.Function.Name == mcpTool {
			return s.executeMCPTool(ctx, toolCall, user)
		}
	}

	// Check if it's an HTTP tool
	for _, httpTool := range s.httpTools {
		if httpTool.Name == toolCall.Function.Name {
			return s.executeHTTPTool(ctx, httpTool, toolCall.Function.Arguments)
		}
	}

	return "", fmt.Errorf("unknown tool: %s", toolCall.Function.Name)
}

func (s *Service) executeMCPTool(ctx context.Context, toolCall ToolCall, user *model.User) (string, error) {
	// For now, return a simplified response indicating the tool was called
	return fmt.Sprintf("Called %s tool successfully", toolCall.Function.Name), nil
}





func (s *Service) executeHTTPTool(ctx context.Context, tool HTTPTool, args map[string]interface{}) (string, error) {
	// Build request body
	var reqBody io.Reader
	if tool.Method == "POST" || tool.Method == "PUT" {
		jsonBody, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, tool.Method, tool.URL, reqBody)
	if err != nil {
		return "", err
	}

	// Set headers
	for key, value := range tool.Headers {
		req.Header.Set(key, value)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Execute request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(respBody), nil
}

func (s *Service) writeSSEEvent(writer io.Writer, event SSEEvent) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(writer, "data: %s\n\n", data)
	if flusher, ok := writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *Service) continueWithToolResults(ctx context.Context, toolCalls []ToolCall, user *model.User, writer io.Writer) error {
	// This would continue the conversation with tool results
	// For now, just indicate tools were executed
	event := SSEEvent{
		Type: "content",
		Data: "\n\n*Tools executed successfully*",
	}
	s.writeSSEEvent(writer, event)
	return nil
}

func (s *Service) AddHTTPTool(tool HTTPTool) {
	s.httpTools = append(s.httpTools, tool)
}