package chat

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/mcp"
)

//go:embed system-prompt.md
var defaultSystemPrompt string

type Service struct {
	config       config.ChatConfig
	mcpServer    *mcp.Server
	restClient   *rest.RESTClient
	systemPrompt string
	streamState  *streamState
}

func NewService(config config.ChatConfig, mcpServer *mcp.Server, router *http.ServeMux) (*Service, error) {
	restClient, err := rest.NewClient(config.OpenAIBaseURL, config.OpenAIAPIKey, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client: %w", err)
	}
	restClient.SetTimeout(60 * time.Second)
	restClient.SetTokenFormat("Bearer %s")

	// Load system prompt
	systemPrompt := defaultSystemPrompt
	if config.SystemPromptFile != "" {
		content, err := os.ReadFile(config.SystemPromptFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read system prompt file %s: %w", config.SystemPromptFile, err)
		}
		systemPrompt = string(content)
	}

	chatService := &Service{
		config:       config,
		mcpServer:    mcpServer,
		restClient:   restClient,
		systemPrompt: systemPrompt,
	}

	// Chat
	router.HandleFunc("POST /api/chat/stream", middleware.ApiAuth(chatService.HandleChatStream))

	return chatService, nil
}

func (s *Service) streamChat(ctx context.Context, messages []ChatMessage, user *model.User, w http.ResponseWriter, r *http.Request) error {
	if len(messages) == 0 {
		return nil
	}

	sseWriter := rest.NewSSEStreamWriter(w, r)
	defer sseWriter.Close()

	openAIMessages := s.convertMessages(messages)
	tools, err := s.getMCPTools(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to get tools: %w", err)
	}

	req := OpenAIRequest{
		Model:           s.config.Model,
		Messages:        openAIMessages,
		Tools:           tools,
		MaxTokens:       s.config.MaxTokens,
		Temperature:     s.config.Temperature,
		ReasoningEffort: s.config.ReasoningEffort,
		Stream:          true,
	}

	return s.callOpenAIWithContext(ctx, req, user, sseWriter, openAIMessages)
}

func (s *Service) callOpenAIWithContext(ctx context.Context, req OpenAIRequest, user *model.User, sseWriter *rest.SSEStreamWriter, conversationHistory []OpenAIMessage) error {
	return rest.StreamData[*OpenAIResponse, OpenAIResponse](
		s.restClient,
		ctx,
		"POST",
		"chat/completions",
		req,
		func(response *OpenAIResponse) (bool, error) {
			return s.processStreamChunk(ctx, *response, user, sseWriter, conversationHistory)
		},
		rest.StreamSSE,
	)
}

func (s *Service) convertMessages(messages []ChatMessage) []OpenAIMessage {
	var openAIMessages []OpenAIMessage

	// Add system prompt (always first)
	if s.systemPrompt != "" {
		openAIMessages = append(openAIMessages, OpenAIMessage{
			Role:    "system",
			Content: s.systemPrompt,
		})
	}

	// Convert chat messages, skipping any existing system messages from history
	for _, msg := range messages {
		if msg.Role != "system" {
			openAIMessage := OpenAIMessage{
				Role:    msg.Role,
				Content: msg.Content,
			}

			// Include tool calls for assistant messages
			if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
				openAIMessage.ToolCalls = msg.ToolCalls
			}

			// Include tool call ID for tool messages
			if msg.Role == "tool" && msg.ToolCallID != "" {
				openAIMessage.ToolCallID = msg.ToolCallID
			}

			openAIMessages = append(openAIMessages, openAIMessage)
		}
	}

	return openAIMessages
}

type streamState struct {
	toolCallBuffer   map[int]*ToolCall
	argumentsBuffer  map[int]string
	assistantMessage strings.Builder
}

func (s *Service) processStreamChunk(ctx context.Context, response OpenAIResponse, user *model.User, sseWriter *rest.SSEStreamWriter, conversationHistory []OpenAIMessage) (bool, error) {
	if len(response.Choices) == 0 {
		return false, nil
	}

	// Initialize state if not exists (we need to maintain state across chunks)
	if s.streamState == nil {
		s.streamState = &streamState{
			toolCallBuffer:  make(map[int]*ToolCall),
			argumentsBuffer: make(map[int]string),
		}
	}

	choice := response.Choices[0]

	// Handle tool calls
	if len(choice.Delta.ToolCalls) > 0 {
		for _, deltaCall := range choice.Delta.ToolCalls {
			index := deltaCall.Index
			if s.streamState.toolCallBuffer[index] == nil {
				s.streamState.toolCallBuffer[index] = &ToolCall{
					Index:    index,
					Function: ToolCallFunction{Arguments: make(map[string]interface{})},
				}
				s.streamState.argumentsBuffer[index] = ""
			}
			if deltaCall.ID != "" {
				s.streamState.toolCallBuffer[index].ID = deltaCall.ID
			}
			if deltaCall.Type != "" {
				s.streamState.toolCallBuffer[index].Type = deltaCall.Type
			}
			if deltaCall.Function.Name != "" {
				s.streamState.toolCallBuffer[index].Function.Name = deltaCall.Function.Name
			}
			if deltaCall.Function.Arguments != "" {
				s.streamState.argumentsBuffer[index] += deltaCall.Function.Arguments
			}
		}
	}

	// Handle content
	if choice.Delta.Content != "" {
		s.streamState.assistantMessage.WriteString(choice.Delta.Content)
		sseWriter.WriteChunk(SSEEvent{
			Type: "content",
			Data: choice.Delta.Content,
		})
	}

	// Handle finish reason
	if choice.FinishReason == "tool_calls" {
		// Parse and validate arguments before processing
		for _, toolCall := range s.streamState.toolCallBuffer {
			if argsStr, exists := s.streamState.argumentsBuffer[toolCall.Index]; exists {
				if argsStr == "" || argsStr == "null" {
					toolCall.Function.Arguments = make(map[string]interface{})
				} else {
					if err := json.Unmarshal([]byte(argsStr), &toolCall.Function.Arguments); err != nil {
						toolCall.Function.Arguments = make(map[string]interface{})
					}
				}
			}
		}

		err := s.handleToolCalls(ctx, s.streamState, user, sseWriter, conversationHistory)
		s.streamState = nil // Reset state after tool calls
		return true, err
	}

	// Check if we're done
	if choice.FinishReason != "" && choice.FinishReason != "tool_calls" {
		// Send done event
		sseWriter.WriteChunk(SSEEvent{
			Type: "done",
			Data: nil,
		})
		s.streamState = nil // Reset state
		return true, nil
	}

	return false, nil
}

func (s *Service) handleToolCalls(ctx context.Context, state *streamState, user *model.User, sseWriter *rest.SSEStreamWriter, conversationHistory []OpenAIMessage) error {
	var toolCalls []ToolCall
	var toolResults []ToolResult

	// Send tool calls info to frontend
	for _, toolCall := range state.toolCallBuffer {
		if toolCall != nil && toolCall.Function.Name != "" {
			if toolCall.ID == "" {
				toolCall.ID = fmt.Sprintf("call_%d", toolCall.Index)
			}
			toolCalls = append(toolCalls, *toolCall)
		}
	}

	if len(toolCalls) > 0 {
		sseWriter.WriteChunk(SSEEvent{
			Type: "tool_calls",
			Data: toolCalls,
		})
	}

	// Execute tools and collect results
	for _, toolCall := range toolCalls {
		result, err := s.executeMCPTool(ctx, toolCall, user)
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
				"tool_name":    toolCall.Function.Name,
				"result":       result,
				"tool_call_id": toolCall.ID,
			},
		})
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

	tools, err := s.getMCPTools(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to get tools: %w", err)
	}

	req := OpenAIRequest{
		Model:           s.config.Model,
		Messages:        newHistory,
		Tools:           tools,
		MaxTokens:       s.config.MaxTokens,
		Temperature:     s.config.Temperature,
		ReasoningEffort: s.config.ReasoningEffort,
		Stream:          true,
	}

	s.streamState = nil // Reset state after tool calls

	return s.callOpenAIWithContext(ctx, req, user, sseWriter, newHistory)
}

func (s *Service) getMCPTools(ctx context.Context, user *model.User) ([]OpenAITool, error) {
	if s.mcpServer == nil {
		return []OpenAITool{}, nil
	}

	// Get tools directly from MCP server without caching
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

func (s *Service) executeMCPTool(ctx context.Context, toolCall ToolCall, user *model.User) (string, error) {
	if s.mcpServer == nil {
		return "", fmt.Errorf("MCP server is not configured")
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

func (s *Service) HandleChatStream(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	if user == nil {
		rest.WriteResponse(http.StatusUnauthorized, w, r, map[string]string{
			"error": "User not found",
		})
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	if len(req.Messages) == 0 {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "No messages provided",
		})
		return
	}

	err := s.streamChat(r.Context(), req.Messages, user, w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
