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

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/mcp"
	"github.com/paularlott/knot/internal/service"
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

		// Handle tool calls
		if len(choice.Delta.ToolCalls) > 0 {
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
	// This is a simplified approach - in practice you'd want to properly call the MCP server
	// For now, we'll return the known tools
	return []OpenAITool{
		{
			Type: "function",
			Function: OpenAIToolFunction{
				Name:        "list_spaces",
				Description: "List all spaces for the current user with their current status",
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
				Description: "Start a space by its ID or name. The space must be in a stopped state.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"space_id": map[string]interface{}{
							"type":        "string",
							"description": "The ID or name of the space to start",
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
				Description: "Stop a space by its ID or name. The space must be in a running state.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"space_id": map[string]interface{}{
							"type":        "string",
							"description": "The ID or name of the space to stop",
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
				Name:        "find_space_by_name",
				Description: "Find a space by its name and return its ID and details",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"space_name": map[string]interface{}{
							"type":        "string",
							"description": "The name of the space to find",
						},
					},
					"required":             []string{"space_name"},
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
	// Check if it's an MCP tool
	mcpTools := []string{"list_spaces", "start_space", "stop_space", "find_space_by_name", "get_docker_podman_spec"}
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
	switch toolCall.Function.Name {
	case "start_space":
		spaceID, ok := toolCall.Function.Arguments["space_id"].(string)
		if !ok {
			return "", fmt.Errorf("space_id is required")
		}

		// Check if the spaceID is actually a space name (common user mistake)
		// Try to find by ID first, if not found, try by name
		db := database.GetInstance()
		_, err := db.GetSpace(spaceID)
		if err != nil {
			// Maybe it's a space name, try to find by name
			spaces, nameErr := db.GetSpacesForUser(user.Id)
			if nameErr != nil {
				return "", fmt.Errorf("failed to get spaces: %v", nameErr)
			}

			var foundSpace *model.Space
			for _, space := range spaces {
				if space.Name == spaceID {
					foundSpace = space
					break
				}
			}

			if foundSpace == nil {
				return "", fmt.Errorf("space not found by ID or name: %s", spaceID)
			}
			spaceID = foundSpace.Id
		}

		return s.startSpace(ctx, spaceID, user)

	case "stop_space":
		spaceID, ok := toolCall.Function.Arguments["space_id"].(string)
		if !ok {
			return "", fmt.Errorf("space_id is required")
		}

		// Same logic for stop_space - handle both ID and name
		db := database.GetInstance()
		_, err := db.GetSpace(spaceID)
		if err != nil {
			// Maybe it's a space name, try to find by name
			spaces, nameErr := db.GetSpacesForUser(user.Id)
			if nameErr != nil {
				return "", fmt.Errorf("failed to get spaces: %v", nameErr)
			}

			var foundSpace *model.Space
			for _, space := range spaces {
				if space.Name == spaceID {
					foundSpace = space
					break
				}
			}

			if foundSpace == nil {
				return "", fmt.Errorf("space not found by ID or name: %s", spaceID)
			}
			spaceID = foundSpace.Id
		}

		return s.stopSpace(ctx, spaceID, user)

	case "list_spaces":
		return s.listSpaces(ctx, user)

	case "find_space_by_name":
		spaceName, ok := toolCall.Function.Arguments["space_name"].(string)
		if !ok {
			return "", fmt.Errorf("space_name is required")
		}
		return s.findSpaceByName(ctx, spaceName, user)

	default:
		return "", fmt.Errorf("unknown MCP tool: %s", toolCall.Function.Name)
	}
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

func (s *Service) startSpace(ctx context.Context, spaceID string, user *model.User) (string, error) {
	db := database.GetInstance()

	// Get the space to verify it exists and user has access
	space, err := db.GetSpace(spaceID)
	if err != nil {
		return "", fmt.Errorf("space not found: %v", err)
	}

	// Check permissions
	if user.Id != space.UserId && user.Id != space.SharedWithUserId && !user.HasPermission(model.PermissionManageSpaces) {
		return "", fmt.Errorf("access denied to space %s", spaceID)
	}

	// Check if space can be started
	if space.IsDeployed || space.IsPending || space.IsDeleting {
		return "", fmt.Errorf("space %s cannot be started (current state: deployed=%v, pending=%v, deleting=%v)", spaceID, space.IsDeployed, space.IsPending, space.IsDeleting)
	}

	// Get template
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		return "", fmt.Errorf("template not found: %v", err)
	}

	// Start the space using the container service
	if err := service.GetContainerService().StartSpace(space, template, user); err != nil {
		return "", fmt.Errorf("failed to start space: %v", err)
	}

	return fmt.Sprintf("Successfully started space '%s' (ID: %s)", space.Name, spaceID), nil
}

func (s *Service) stopSpace(ctx context.Context, spaceID string, user *model.User) (string, error) {
	db := database.GetInstance()

	// Get the space
	space, err := db.GetSpace(spaceID)
	if err != nil {
		return "", fmt.Errorf("space not found: %v", err)
	}

	// Check permissions
	if user.Id != space.UserId && user.Id != space.SharedWithUserId && !user.HasPermission(model.PermissionManageSpaces) {
		return "", fmt.Errorf("access denied to space %s", spaceID)
	}

	// Check if space can be stopped
	if (!space.IsDeployed && !space.IsPending) || space.IsDeleting {
		return "", fmt.Errorf("space %s cannot be stopped (current state: deployed=%v, pending=%v, deleting=%v)", spaceID, space.IsDeployed, space.IsPending, space.IsDeleting)
	}

	// Stop the space
	if err := service.GetContainerService().StopSpace(space); err != nil {
		return "", fmt.Errorf("failed to stop space: %v", err)
	}

	return fmt.Sprintf("Successfully stopped space '%s' (ID: %s)", space.Name, spaceID), nil
}

func (s *Service) listSpaces(ctx context.Context, user *model.User) (string, error) {
	db := database.GetInstance()

	// Get spaces for the user
	spaces, err := db.GetSpacesForUser(user.Id)
	if err != nil {
		return "", fmt.Errorf("failed to get spaces: %v", err)
	}

	if len(spaces) == 0 {
		return "No spaces found for user", nil
	}

	// Format the response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Found %d spaces:\n", len(spaces)))

	for _, space := range spaces {
		if space.IsDeleted {
			continue
		}

		status := "stopped"
		if space.IsDeployed {
			status = "running"
		} else if space.IsPending {
			status = "starting"
		} else if space.IsDeleting {
			status = "deleting"
		}

		result.WriteString(fmt.Sprintf("- %s (ID: %s) - Status: %s\n", space.Name, space.Id, status))
	}

	return result.String(), nil
}

func (s *Service) findSpaceByName(ctx context.Context, spaceName string, user *model.User) (string, error) {
	db := database.GetInstance()

	// Get all spaces for the user and search by name
	spaces, err := db.GetSpacesForUser(user.Id)
	if err != nil {
		return "", fmt.Errorf("failed to get spaces: %v", err)
	}

	var foundSpace *model.Space
	for _, space := range spaces {
		if space.Name == spaceName {
			foundSpace = space
			break
		}
	}

	if foundSpace == nil {
		return fmt.Sprintf("No space found with name '%s'", spaceName), nil
	}

	status := "stopped"
	if foundSpace.IsDeployed {
		status = "running"
	} else if foundSpace.IsPending {
		status = "starting"
	} else if foundSpace.IsDeleting {
		status = "deleting"
	}

	return fmt.Sprintf("Found space '%s' (ID: %s) - Status: %s", foundSpace.Name, foundSpace.Id, status), nil
}

func (s *Service) AddHTTPTool(tool HTTPTool) {
	s.httpTools = append(s.httpTools, tool)
}
