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
	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/mcp"
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

	// Call OpenAI API with conversation context
	return s.callOpenAIWithContext(ctx, req, user, writer, openAIMessages)
}

func (s *Service) callOpenAIWithContext(ctx context.Context, req OpenAIRequest, user *model.User, writer io.Writer, conversationHistory []OpenAIMessage) error {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.config.OpenAIBaseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if s.config.OpenAIAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+s.config.OpenAIAPIKey)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI API error: %d - %s", resp.StatusCode, string(body))
	}

	return s.processStreamResponseWithContext(ctx, resp.Body, user, writer, conversationHistory)
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

func (s *Service) processStreamResponseWithContext(ctx context.Context, reader io.Reader, user *model.User, writer io.Writer, conversationHistory []OpenAIMessage) error {
	scanner := bufio.NewScanner(reader)
	var toolCallBuffer = make(map[int]*ToolCall)
	var argumentsBuffer = make(map[int]string)
	var assistantMessage strings.Builder

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

		fmt.Println("Choices")
		util.PrettyPrintJSON(response)

		// Handle tool calls
		if len(choice.Delta.ToolCalls) > 0 {
			fmt.Println("we have tool calls")
			for _, deltaCall := range choice.Delta.ToolCalls {
				index := deltaCall.Index

				if toolCallBuffer[index] == nil {
					toolCallBuffer[index] = &ToolCall{
						Index: index,
						Function: ToolCallFunction{
							Arguments: make(map[string]interface{}),
						},
					}
					argumentsBuffer[index] = ""
				}

				if deltaCall.ID != "" {
					toolCallBuffer[index].ID = deltaCall.ID
				}
				if deltaCall.Type != "" {
					toolCallBuffer[index].Type = deltaCall.Type
				}
				if deltaCall.Function.Name != "" {
					toolCallBuffer[index].Function.Name = deltaCall.Function.Name
				}
				if deltaCall.Function.Arguments != "" {
					argumentsBuffer[index] += deltaCall.Function.Arguments
				}
			}
		}

		// Handle content
		if choice.Delta.Content != "" {
			assistantMessage.WriteString(choice.Delta.Content)
			event := SSEEvent{
				Type: "content",
				Data: choice.Delta.Content,
			}
			s.writeSSEEvent(writer, event)
		}

		// Handle finish reason
		if choice.FinishReason == "tool_calls" {
			// Execute tools and continue conversation
			var toolCalls []ToolCall
			var toolResults []ToolResult

			fmt.Println("tool buffer", toolCallBuffer)

			for index, toolCall := range toolCallBuffer {
				if toolCall != nil && toolCall.Function.Name != "" {
					// Parse accumulated JSON arguments
					if argumentsBuffer[index] != "" {
						var parsedArgs map[string]interface{}
						if err := json.Unmarshal([]byte(argumentsBuffer[index]), &parsedArgs); err == nil {
							toolCall.Function.Arguments = parsedArgs
						}
					}

					toolCalls = append(toolCalls, *toolCall)

					// Execute tool
					fmt.Println("at tool call", toolCall)
					result, err := s.executeToolCall(ctx, *toolCall, user)
					if err != nil {
						result = fmt.Sprintf("Error executing tool: %v", err)
					}

					toolResults = append(toolResults, ToolResult{
						ToolCallID: toolCall.ID,
						Content:    result,
					})

					// Send tool result event to frontend
					event := SSEEvent{
						Type: "tool_result",
						Data: map[string]interface{}{
							"tool_name": toolCall.Function.Name,
							"result":    result,
						},
					}
					s.writeSSEEvent(writer, event)
				}
			}

			// Build new conversation history with tool calls and results
			newHistory := append(conversationHistory, OpenAIMessage{
				Role:      "assistant",
				Content:   assistantMessage.String(),
				ToolCalls: toolCalls,
			})

			// Add tool results
			for _, result := range toolResults {
				newHistory = append(newHistory, OpenAIMessage{
					Role:       "tool",
					Content:    result.Content,
					ToolCallID: result.ToolCallID,
				})
			}

			// Get available tools
			tools, err := s.getAvailableTools(ctx, user)
			if err != nil {
				return fmt.Errorf("failed to get tools: %w", err)
			}

			// Continue conversation
			req := OpenAIRequest{
				Model:       s.config.Model,
				Messages:    newHistory,
				Tools:       tools,
				MaxTokens:   s.config.MaxTokens,
				Temperature: s.config.Temperature,
				Stream:      true,
			}

			return s.callOpenAIWithContext(ctx, req, user, writer, newHistory)
		}
	}

	return scanner.Err()
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
	if s.mcpServer == nil {
		return []OpenAITool{}, nil
	}

	// Get tools directly from MCP server
	tools := s.mcpServer.ListTools()
	var openAITools []OpenAITool

	for _, tool := range tools {
		openAITools = append(openAITools, OpenAITool{
			Type: "function",
			Function: OpenAIToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema.(map[string]interface{}),
			},
		})
	}

	return openAITools, nil
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
	if s.config.OpenAIAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+s.config.OpenAIAPIKey)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI API error: %d - %s", resp.StatusCode, string(body))
	}

	return s.processStreamResponse(ctx, resp.Body, user, writer)
}

func (s *Service) processStreamResponse(ctx context.Context, reader io.Reader, user *model.User, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	var toolCallBuffer = make(map[int]*ToolCall)
	var argumentsBuffer = make(map[int]string) // Buffer for JSON string arguments

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

		// Handle tool calls - they come in chunks
		if len(choice.Delta.ToolCalls) > 0 {
			for _, deltaCall := range choice.Delta.ToolCalls {
				index := deltaCall.Index

				// Initialize tool call if not exists
				if toolCallBuffer[index] == nil {
					toolCallBuffer[index] = &ToolCall{
						Index: index,
						Function: ToolCallFunction{
							Arguments: make(map[string]interface{}),
						},
					}
					argumentsBuffer[index] = ""
				}

				// Update fields if they exist in this chunk
				if deltaCall.ID != "" {
					toolCallBuffer[index].ID = deltaCall.ID
				}
				if deltaCall.Type != "" {
					toolCallBuffer[index].Type = deltaCall.Type
				}
				if deltaCall.Function.Name != "" {
					toolCallBuffer[index].Function.Name = deltaCall.Function.Name
				}

				// Accumulate function arguments (they come as JSON string chunks)
				if deltaCall.Function.Arguments != "" {
					argumentsBuffer[index] += deltaCall.Function.Arguments
				}
			}
		}

		// Handle content - only stream if we're not about to execute tools
		if choice.Delta.Content != "" && choice.FinishReason != "tool_calls" {
			event := SSEEvent{
				Type: "content",
				Data: choice.Delta.Content,
			}
			s.writeSSEEvent(writer, event)
		}

		// Handle finish reason
		if choice.FinishReason == "tool_calls" {
			// Parse accumulated arguments and execute tools
			var toolCalls []ToolCall
			var toolResults []ToolResult

			for index, toolCall := range toolCallBuffer {
				if toolCall != nil && toolCall.Function.Name != "" {
					// Parse accumulated JSON arguments
					if argumentsBuffer[index] != "" {
						var parsedArgs map[string]interface{}
						if err := json.Unmarshal([]byte(argumentsBuffer[index]), &parsedArgs); err == nil {
							toolCall.Function.Arguments = parsedArgs
						}
					}

					toolCalls = append(toolCalls, *toolCall)

					// Execute tool
					result, err := s.executeToolCall(ctx, *toolCall, user)
					if err != nil {
						result = fmt.Sprintf("Error executing tool: %v", err)
					}

					toolResults = append(toolResults, ToolResult{
						ToolCallID: toolCall.ID,
						Content:    result,
					})

					// Send tool result event to frontend
					event := SSEEvent{
						Type: "tool_result",
						Data: map[string]interface{}{
							"tool_name": toolCall.Function.Name,
							"result":    result,
						},
					}
					s.writeSSEEvent(writer, event)
				}
			}

			// Continue conversation with tool results
			return s.continueWithToolResults(ctx, toolCalls, toolResults, user, writer)
		}
	}

	return scanner.Err()
}

func (s *Service) executeToolCall(ctx context.Context, toolCall ToolCall, user *model.User) (string, error) {

	fmt.Println("Executing tool", toolCall.Function.Name, toolCall.Function.Arguments)

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
	if s.mcpServer == nil {
		return "", fmt.Errorf("MCP server not available")
	}

	// Add user to context for MCP server
	ctxWithUser := context.WithValue(ctx, "user", user)

	// Call tool directly using MCP server's CallTool method
	response, err := s.mcpServer.CallTool(ctxWithUser, toolCall.Function.Name, toolCall.Function.Arguments)
	if err != nil {
		return "", fmt.Errorf("MCP tool call failed: %v", err)
	}

	// Extract text content from response
	if len(response.Content) > 0 && response.Content[0].Type == "text" {
		return response.Content[0].Text, nil
	}

	return "Tool executed successfully", nil
}



func (s *Service) continueWithToolResults(ctx context.Context, toolCalls []ToolCall, toolResults []ToolResult, user *model.User, writer io.Writer) error {
	// Get the original conversation context - we need to rebuild the message history
	// For now, we'll create a simplified continuation
	messages := []OpenAIMessage{
		{
			Role:      "assistant",
			ToolCalls: toolCalls,
		},
	}

	// Add tool results as tool messages
	for _, result := range toolResults {
		messages = append(messages, OpenAIMessage{
			Role:       "tool",
			Content:    result.Content,
			ToolCallID: result.ToolCallID,
		})
	}

	// Get available tools again
	tools, err := s.getAvailableTools(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to get tools: %w", err)
	}

	// Create new request to continue the conversation
	req := OpenAIRequest{
		Model:       s.config.Model,
		Messages:    messages,
		Tools:       tools,
		MaxTokens:   s.config.MaxTokens,
		Temperature: s.config.Temperature,
		Stream:      true,
	}

	return s.callOpenAI(ctx, req, user, writer)
}

func (s *Service) executeHTTPTool(ctx context.Context, tool HTTPTool, args map[string]interface{}) (string, error) {
	var reqBody io.Reader
	if tool.Method == "POST" || tool.Method == "PUT" {
		jsonBody, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, tool.Method, tool.URL, reqBody)
	if err != nil {
		return "", err
	}

	for key, value := range tool.Headers {
		req.Header.Set(key, value)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

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

func (s *Service) AddHTTPTool(tool HTTPTool) {
	s.httpTools = append(s.httpTools, tool)
}
