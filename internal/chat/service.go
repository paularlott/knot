package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/mcp"
)

type Service struct {
	config     ChatConfig
	mcpServer  *mcp.Server
	httpTools  []HTTPTool
	restClient *rest.RESTClient
}

func NewService(config ChatConfig, mcpServer *mcp.Server) (*Service, error) {
	restClient, err := rest.NewClient(config.OpenAIBaseURL, config.OpenAIAPIKey, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client: %w", err)
	}
	restClient.SetTimeout(60 * time.Second)
	restClient.SetTokenFormat("Bearer %s")

	return &Service{
		config:     config,
		mcpServer:  mcpServer,
		httpTools:  []HTTPTool{},
		restClient: restClient,
	}, nil
}

func (s *Service) StreamChat(ctx context.Context, messages []ChatMessage, user *model.User, w http.ResponseWriter, r *http.Request) error {
	if len(messages) == 0 {
		return nil
	}

	sseWriter := rest.NewSSEStreamWriter(w, r)
	defer sseWriter.Close()

	openAIMessages := s.convertMessages(messages)
	tools, err := s.getAvailableTools(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to get tools: %w", err)
	}

	req := OpenAIRequest{
		Model:       s.config.Model,
		Messages:    openAIMessages,
		Tools:       tools,
		MaxTokens:   s.config.MaxTokens,
		Temperature: s.config.Temperature,
		Stream:      true,
	}

	return s.callOpenAIWithContext(ctx, req, user, sseWriter, openAIMessages)
}

func (s *Service) callOpenAIWithContext(ctx context.Context, req OpenAIRequest, user *model.User, sseWriter *rest.SSEStreamWriter, conversationHistory []OpenAIMessage) error {
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

	resp, err := s.restClient.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI API error: %d - %s", resp.StatusCode, string(body))
	}

	return s.processStreamResponse(ctx, resp.Body, user, sseWriter, conversationHistory)
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

type streamState struct {
	toolCallBuffer  map[int]*ToolCall
	argumentsBuffer map[int]string
	assistantMessage strings.Builder
}

func (s *Service) processStreamResponse(ctx context.Context, reader io.Reader, user *model.User, sseWriter *rest.SSEStreamWriter, conversationHistory []OpenAIMessage) error {
	scanner := bufio.NewScanner(reader)
	state := &streamState{
		toolCallBuffer:  make(map[int]*ToolCall),
		argumentsBuffer: make(map[int]string),
	}

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
		util.PrettyPrintJSON(response)

		// Handle tool calls
		if len(choice.Delta.ToolCalls) > 0 {
			for _, deltaCall := range choice.Delta.ToolCalls {
				index := deltaCall.Index
				if state.toolCallBuffer[index] == nil {
					state.toolCallBuffer[index] = &ToolCall{
						Index: index,
						Function: ToolCallFunction{Arguments: make(map[string]interface{})},
					}
					state.argumentsBuffer[index] = ""
				}
				if deltaCall.ID != "" {
					state.toolCallBuffer[index].ID = deltaCall.ID
				}
				if deltaCall.Type != "" {
					state.toolCallBuffer[index].Type = deltaCall.Type
				}
				if deltaCall.Function.Name != "" {
					state.toolCallBuffer[index].Function.Name = deltaCall.Function.Name
				}
				if deltaCall.Function.Arguments != "" {
					state.argumentsBuffer[index] += deltaCall.Function.Arguments
				}
			}
		}

		// Handle content
		if choice.Delta.Content != "" {
			state.assistantMessage.WriteString(choice.Delta.Content)
			sseWriter.WriteChunk(SSEEvent{
				Type: "content",
				Data: choice.Delta.Content,
			})
		}

		// Handle finish reason
		if choice.FinishReason == "tool_calls" {
			return s.handleToolCalls(ctx, state, user, sseWriter, conversationHistory)
		}
	}

	// Send done event
	sseWriter.WriteChunk(SSEEvent{
		Type: "done",
		Data: nil,
	})

	return scanner.Err()
}

func (s *Service) handleToolCalls(ctx context.Context, state *streamState, user *model.User, sseWriter *rest.SSEStreamWriter, conversationHistory []OpenAIMessage) error {
	var toolCalls []ToolCall
	var toolResults []ToolResult

	for index, toolCall := range state.toolCallBuffer {
		if toolCall != nil && toolCall.Function.Name != "" {
			if toolCall.ID == "" {
				toolCall.ID = fmt.Sprintf("call_%d", index)
			}

			if state.argumentsBuffer[index] != "" {
				var parsedArgs map[string]interface{}
				if err := json.Unmarshal([]byte(state.argumentsBuffer[index]), &parsedArgs); err == nil {
					toolCall.Function.Arguments = parsedArgs
				}
			} else {
				toolCall.Function.Arguments = make(map[string]interface{})
			}

			toolCalls = append(toolCalls, *toolCall)

			result, err := s.executeToolCall(ctx, *toolCall, user)
			if err != nil {
				result = fmt.Sprintf("Error executing tool: %v", err)
			}

			toolResults = append(toolResults, ToolResult{
				ToolCallID: toolCall.ID,
				Content:    result,
			})

			sseWriter.WriteChunk(SSEEvent{
				Type: "tool_result",
				Data: map[string]interface{}{
					"tool_name": toolCall.Function.Name,
					"result":    result,
				},
			})
		}
	}

	// Build new conversation history
	newHistory := append(conversationHistory, OpenAIMessage{
		Role:      "assistant",
		Content:   state.assistantMessage.String(),
		ToolCalls: toolCalls,
	})

	for _, result := range toolResults {
		newHistory = append(newHistory, OpenAIMessage{
			Role:       "tool",
			Content:    result.Content,
			ToolCallID: result.ToolCallID,
		})
	}

	tools, err := s.getAvailableTools(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to get tools: %w", err)
	}

	req := OpenAIRequest{
		Model:       s.config.Model,
		Messages:    newHistory,
		Tools:       tools,
		MaxTokens:   s.config.MaxTokens,
		Temperature: s.config.Temperature,
		Stream:      true,
	}

	return s.callOpenAIWithContext(ctx, req, user, sseWriter, newHistory)
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

func (s *Service) executeHTTPTool(ctx context.Context, tool HTTPTool, args map[string]interface{}) (string, error) {
	// Parse the URL to separate base and path
	u, err := url.Parse(tool.URL)
	if err != nil {
		return "", fmt.Errorf("invalid tool URL: %w", err)
	}

	baseURL := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	path := u.Path
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}

	toolClient, err := rest.NewClient(baseURL, "", false)
	if err != nil {
		return "", fmt.Errorf("failed to create tool client: %w", err)
	}
	toolClient.SetTimeout(30 * time.Second)

	for key, value := range tool.Headers {
		toolClient.SetHeader(key, value)
	}

	var result string
	switch tool.Method {
	case "GET":
		_, err = toolClient.Get(ctx, path, &result)
	case "POST":
		_, err = toolClient.Post(ctx, path, args, &result, 0)
	case "PUT":
		_, err = toolClient.Put(ctx, path, args, &result, 0)
	case "DELETE":
		_, err = toolClient.Delete(ctx, path, args, &result, 0)
	default:
		return "", fmt.Errorf("unsupported HTTP method: %s", tool.Method)
	}

	return result, err
}



func (s *Service) AddHTTPTool(tool HTTPTool) {
	s.httpTools = append(s.httpTools, tool)
}

func (s *Service) UpdateConfig(config ChatConfig) error {
	s.config = config
	restClient, err := rest.NewClient(config.OpenAIBaseURL, config.OpenAIAPIKey, false)
	if err != nil {
		return fmt.Errorf("failed to create REST client: %w", err)
	}
	restClient.SetTimeout(60 * time.Second)
	restClient.SetTokenFormat("Bearer %s")
	s.restClient = restClient
	return nil
}
