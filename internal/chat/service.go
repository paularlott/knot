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
	"github.com/rs/zerolog/log"

	"github.com/paularlott/mcp"
)

//go:embed system-prompt.md
var defaultSystemPrompt string

const (
	// Maximum number of tool call iterations to prevent infinite loops
	MAX_TOOL_CALL_ITERATIONS = 20

	CONTENT_BATCH_SIZE = 150                    // characters - smaller for more responsive streaming
	CONTENT_BATCH_TIME = 100 * time.Millisecond // shorter time for more responsive streaming
)

type Service struct {
	config       config.ChatConfig
	mcpServer    *mcp.Server
	restClient   *rest.RESTClient
	systemPrompt string
}

type streamState struct {
	toolCallBuffer  map[int]*ToolCall
	argumentsBuffer map[int]string
	inThinking      bool
	inReasoning     bool
	contentBuffer   strings.Builder // Content batching
	lastFlushTime   time.Time
}

func NewService(config config.ChatConfig, mcpServer *mcp.Server, router *http.ServeMux) (*Service, error) {
	restClient, err := rest.NewClient(config.OpenAIBaseURL, config.OpenAIAPIKey, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client: %w", err)
	}
	restClient.SetTimeout(5 * time.Minute)
	restClient.SetTokenFormat("Bearer %s")
	restClient.SetContentType(rest.ContentTypeJSON)
	restClient.SetAccept(rest.ContentTypeJSON)

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
	router.HandleFunc("POST /api/chat/stream", middleware.ApiAuth(middleware.ApiPermissionUseWebAssistant(chatService.HandleChatStream)))

	return chatService, nil
}

func (s *Service) streamChat(ctx context.Context, messages []ChatMessage, user *model.User, w http.ResponseWriter, r *http.Request) error {
	if len(messages) == 0 {
		log.Warn().Str("user_id", user.Id).Msg("streamChat: No messages provided")
		return nil
	}

	// Check if client disconnected before starting
	select {
	case <-ctx.Done():
		log.Debug().Str("user_id", user.Id).Msg("streamChat: Client disconnected before processing")
		return ctx.Err()
	default:
	}

	sseWriter := rest.NewStreamWriter(w, r)
	defer sseWriter.Close()

	currentMessages := s.convertMessages(messages)

	tools, err := s.getMCPTools(user)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.Id).Msg("streamChat: Failed to get MCP tools")
		sseWriter.WriteChunk(SSEEvent{
			Type: "error",
			Data: map[string]string{"error": "Failed to load available tools. Please try again."},
		})
		return fmt.Errorf("failed to get tools: %w", err)
	}

	// If no tools are available, log a warning
	if len(tools) == 0 {
		log.Warn().Str("user_id", user.Id).Msg("streamChat: No tools available - LLM will not be able to use tools")
	}

	// Iterative tool call loop
	for iteration := range MAX_TOOL_CALL_ITERATIONS {
		// Check if client disconnected
		select {
		case <-ctx.Done():
			log.Debug().Str("user_id", user.Id).Int("iteration", iteration).Msg("streamChat: Client disconnected during iteration")
			return ctx.Err()
		default:
		}

		req := OpenAIRequest{
			Model:           s.config.Model,
			Messages:        currentMessages,
			Tools:           tools,
			MaxTokens:       s.config.MaxTokens,
			Temperature:     s.config.Temperature,
			ReasoningEffort: s.config.ReasoningEffort,
			Stream:          true,
		}

		log.Debug().Str("user_id", user.Id).Str("model", s.config.Model).Bool("has_tools", len(tools) > 0).Int("iteration", iteration).Msg("streamChat: Calling OpenAI API")

		// Call OpenAI API with retry mechanism
		toolCalls, assistantContent, err := s.callOpenAIWithRetry(ctx, req, user, sseWriter, iteration)
		if err != nil {
			log.Error().Err(err).Str("user_id", user.Id).Int("iteration", iteration).Msg("streamChat: OpenAI API call failed")
			sseWriter.WriteChunk(SSEEvent{
				Type: "error",
				Data: map[string]string{"error": s.formatUserFriendlyError(err)},
			})
			return err
		}

		// If no tool calls, we're done
		if len(toolCalls) == 0 {
			log.Debug().Str("user_id", user.Id).Int("iteration", iteration).Msg("streamChat: No tool calls, conversation complete")
			sseWriter.WriteChunk(SSEEvent{
				Type: "done",
				Data: nil,
			})
			return nil
		}

		// Check if client disconnected before tool execution
		select {
		case <-ctx.Done():
			log.Debug().Str("user_id", user.Id).Int("iteration", iteration).Msg("streamChat: Client disconnected before tool execution")
			return ctx.Err()
		default:
		}

		// Execute tools and get results
		toolResults, err := s.executeToolCalls(ctx, toolCalls, user, sseWriter)
		if err != nil {
			log.Error().Err(err).Str("user_id", user.Id).Int("iteration", iteration).Msg("streamChat: Tool execution failed")
			return err
		}

		// Add assistant message with tool calls to conversation
		currentMessages = append(currentMessages, OpenAIMessage{
			Role:      "assistant",
			Content:   assistantContent,
			ToolCalls: toolCalls,
		})

		// Add tool results to conversation
		for _, result := range toolResults {
			currentMessages = append(currentMessages, OpenAIMessage{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: result.ToolCallID,
			})
		}

	}

	// If we reach here, we've hit the iteration limit
	log.Warn().Str("user_id", user.Id).Int("max_iterations", MAX_TOOL_CALL_ITERATIONS).Msg("streamChat: Maximum tool call iterations reached")
	sseWriter.WriteChunk(SSEEvent{
		Type: "error",
		Data: map[string]string{"error": "Maximum tool call limit reached. The conversation has been stopped to prevent infinite loops."},
	})
	return fmt.Errorf("maximum tool call iterations (%d) reached", MAX_TOOL_CALL_ITERATIONS)
}

// callOpenAIWithRetry calls OpenAI API with retry mechanism and returns tool calls and assistant content
func (s *Service) callOpenAIWithRetry(ctx context.Context, req OpenAIRequest, user *model.User, sseWriter *rest.StreamWriter, iteration int) ([]ToolCall, string, error) {
	maxRetries := 2
	var err error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Warn().Str("user_id", user.Id).Int("attempt", attempt+1).Int("iteration", iteration).Msg("callOpenAIWithRetry: Retrying OpenAI API call")
			// Brief delay before retry
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		toolCalls, assistantContent, err := s.callOpenAIStream(ctx, req, user, sseWriter)
		if err == nil {
			return toolCalls, assistantContent, nil // Success
		}

		// Check if this is a retryable error
		if !s.isRetryableError(err) {
			break // Don't retry for non-retryable errors
		}

		log.Warn().Err(err).Str("user_id", user.Id).Int("attempt", attempt+1).Int("iteration", iteration).Msg("callOpenAIWithRetry: Retryable error occurred")
	}

	return nil, "", err
}

// callOpenAIStream makes a single call to OpenAI API and processes the stream
func (s *Service) callOpenAIStream(ctx context.Context, req OpenAIRequest, user *model.User, sseWriter *rest.StreamWriter) ([]ToolCall, string, error) {

	// Initialize stream state for this call
	streamState := &streamState{
		toolCallBuffer:  make(map[int]*ToolCall),
		argumentsBuffer: make(map[int]string),
		inThinking:      false,
		inReasoning:     false,
		lastFlushTime:   time.Now(),
	}

	var toolCalls []ToolCall
	var assistantContent strings.Builder

	err := rest.StreamData(
		s.restClient,
		ctx,
		"POST",
		"chat/completions",
		req,
		func(response *OpenAIResponse) (bool, error) {
			// Check if client disconnected
			select {
			case <-ctx.Done():
				log.Debug().Str("user_id", user.Id).Msg("callOpenAIStream: Client disconnected during stream processing")
				return true, ctx.Err() // Return true to stop streaming
			default:
			}

			if response == nil {
				log.Error().Str("user_id", user.Id).Msg("callOpenAIStream: Received nil response from OpenAI")
				return false, fmt.Errorf("received nil response from AI service")
			}

			// Log any errors in the response
			if len(response.Choices) > 0 && response.Choices[0].FinishReason == "error" {
				log.Error().Str("user_id", user.Id).Msg("callOpenAIStream: OpenAI returned error finish reason")
			}

			return s.processStreamChunkIterative(ctx, *response, user, sseWriter, streamState, &assistantContent, &toolCalls)
		},
	)

	// Flush any remaining content in the buffer
	if streamState.inReasoning {
		s.addToContentBuffer("</think>", streamState, sseWriter)
		streamState.inReasoning = false
	}
	if err := s.flushContentBuffer(streamState, sseWriter); err != nil {
		return nil, "", err
	}

	if err != nil {
		log.Error().Err(err).Str("user_id", user.Id).Msg("callOpenAIStream: OpenAI stream failed")
		return nil, "", err
	}

	return toolCalls, assistantContent.String(), nil
}

func (s *Service) convertMessages(messages []ChatMessage) []OpenAIMessage {
	openAIMessages := make([]OpenAIMessage, 0, len(messages)+1)

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

// processStreamChunkIterative processes stream chunks for the iterative approach
func (s *Service) processStreamChunkIterative(ctx context.Context, response OpenAIResponse, user *model.User, sseWriter *rest.StreamWriter, streamState *streamState, assistantContent *strings.Builder, toolCalls *[]ToolCall) (bool, error) {
	if len(response.Choices) == 0 {
		return false, nil
	}

	choice := response.Choices[0]

	// Handle tool calls
	if len(choice.Delta.ToolCalls) > 0 {
		for _, deltaCall := range choice.Delta.ToolCalls {
			index := deltaCall.Index
			if streamState.toolCallBuffer[index] == nil {
				streamState.toolCallBuffer[index] = &ToolCall{
					Index:    index,
					Function: ToolCallFunction{Arguments: make(map[string]any)},
				}
				streamState.argumentsBuffer[index] = ""
			}
			if deltaCall.ID != "" {
				streamState.toolCallBuffer[index].ID = deltaCall.ID
			}
			if deltaCall.Type != "" {
				streamState.toolCallBuffer[index].Type = deltaCall.Type
			}
			if deltaCall.Function.Name != "" {
				streamState.toolCallBuffer[index].Function.Name = deltaCall.Function.Name
			}
			if deltaCall.Function.Arguments != "" {
				streamState.argumentsBuffer[index] += deltaCall.Function.Arguments
			}
		}
	}

	// Handle content and reasoning content
	var contentToSend string
	if choice.Delta.Content != "" {
		if streamState.inReasoning {
			// Wrap content in think tags for frontend processing
			s.addToContentBuffer("</think>", streamState, sseWriter)
			streamState.inReasoning = false
		}

		contentToSend = choice.Delta.Content
		if contentToSend == "<think>" {
			streamState.inThinking = true
		} else if contentToSend == "</think>" {
			streamState.inThinking = false
		} else if !streamState.inThinking && len(contentToSend) > 0 {
			assistantContent.WriteString(choice.Delta.Content)
		}
	} else if choice.Delta.ReasoningContent != "" {
		if !streamState.inReasoning {
			// Wrap reasoning content in think tags for frontend processing
			s.addToContentBuffer("<think>", streamState, sseWriter)
			streamState.inReasoning = true
		}

		contentToSend = choice.Delta.ReasoningContent
	}

	err := s.addToContentBuffer(contentToSend, streamState, sseWriter)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.Id).Msg("processStreamChunkIterative: Failed to write content chunk")
		return false, fmt.Errorf("failed to write content to stream: %w", err)
	}

	// Handle finish reason
	if choice.FinishReason == "tool_calls" {
		log.Debug().Str("user_id", user.Id).Str("finish_reason", "tool_calls").Msg("Stream finished with tool_calls")

		if streamState.inReasoning {
			s.addToContentBuffer("</think>", streamState, sseWriter)
			streamState.inReasoning = false
		}

		if err := s.flushContentBuffer(streamState, sseWriter); err != nil {
			return false, err
		}

		log.Debug().Str("user_id", user.Id).Int("buffered_tools", len(streamState.toolCallBuffer)).Msg("processStreamChunkIterative: LLM finished with tool_calls reason")
		return s.finalizeToolCalls(streamState, toolCalls, user)
	}

	// Check if we're done with other finish reasons
	if choice.FinishReason != "" && choice.FinishReason != "tool_calls" {
		log.Debug().Str("user_id", user.Id).Str("finish_reason", choice.FinishReason).Msg("Stream finished with reason")

		if streamState.inReasoning {
			s.addToContentBuffer("</think>", streamState, sseWriter)
			streamState.inReasoning = false
		}
		if err := s.flushContentBuffer(streamState, sseWriter); err != nil {
			return false, err
		}

		// If we have tool calls buffered but the finish reason is not "tool_calls",
		// this is likely a Gemini API quirk - let's process the tools anyway
		if len(streamState.toolCallBuffer) > 0 {
			log.Debug().Str("user_id", user.Id).Msg("processStreamChunkIterative: Processing buffered tool calls despite finish reason")
			return s.finalizeToolCalls(streamState, toolCalls, user)
		}

		// No tool calls, we're done
		return true, nil
	}

	return false, nil
}

// finalizeToolCalls processes buffered tool calls and prepares them for execution
func (s *Service) finalizeToolCalls(streamState *streamState, toolCalls *[]ToolCall, user *model.User) (bool, error) {
	// Parse and validate arguments before processing
	for _, toolCall := range streamState.toolCallBuffer {
		if argsStr, exists := streamState.argumentsBuffer[toolCall.Index]; exists {
			if argsStr == "" || argsStr == "null" {
				toolCall.Function.Arguments = make(map[string]any)
			} else {
				if err := json.Unmarshal([]byte(argsStr), &toolCall.Function.Arguments); err != nil {
					log.Error().Err(err).Str("user_id", user.Id).Str("tool_name", toolCall.Function.Name).Msg("finalizeToolCalls: Failed to parse tool arguments")
					toolCall.Function.Arguments = make(map[string]any)
				}
			}
		}
	}

	// Convert buffered tool calls to slice
	for _, toolCall := range streamState.toolCallBuffer {
		if toolCall != nil && toolCall.Function.Name != "" {
			if toolCall.ID == "" {
				toolCall.ID = fmt.Sprintf("call_%d", toolCall.Index)
			}
			*toolCalls = append(*toolCalls, *toolCall)
		}
	}

	return true, nil
}

// executeToolCalls executes all tool calls and returns results
func (s *Service) executeToolCalls(ctx context.Context, toolCalls []ToolCall, user *model.User, sseWriter *rest.StreamWriter) ([]ToolResult, error) {
	var toolResults []ToolResult

	if len(toolCalls) > 0 {
		log.Debug().Str("user_id", user.Id).Int("tool_count", len(toolCalls)).Msg("executeToolCalls: Sending tool calls to frontend")
		sseWriter.WriteChunk(SSEEvent{
			Type: "tool_calls",
			Data: toolCalls,
		})
	} else {
		return toolResults, nil
	}

	// Execute tools and collect results
	for _, toolCall := range toolCalls {
		log.Debug().Str("user_id", user.Id).Str("tool_name", toolCall.Function.Name).Str("tool_call_id", toolCall.ID).Msg("executeToolCalls: Executing tool")
		result, err := s.executeMCPTool(ctx, toolCall, user)
		if err != nil {
			log.Error().Err(err).Str("user_id", user.Id).Str("tool_name", toolCall.Function.Name).Msg("executeToolCalls: Tool execution failed")
			result = fmt.Sprintf("Error executing tool %s: %s", toolCall.Function.Name, s.formatUserFriendlyError(err))
		}

		toolResults = append(toolResults, ToolResult{
			ToolCallID: toolCall.ID,
			Content:    result,
		})

		writeErr := sseWriter.WriteChunk(SSEEvent{
			Type: "tool_result",
			Data: map[string]any{
				"tool_name":    toolCall.Function.Name,
				"result":       result,
				"tool_call_id": toolCall.ID,
			},
		})

		if writeErr != nil {
			log.Error().Err(writeErr).Str("user_id", user.Id).Str("tool_name", toolCall.Function.Name).Msg("executeToolCalls: Failed to write tool result")
			return nil, fmt.Errorf("failed to write tool result to stream: %w", writeErr)
		}
	}

	return toolResults, nil
}

func (s *Service) getMCPTools(user *model.User) ([]OpenAITool, error) {
	if s.mcpServer == nil {
		return []OpenAITool{}, nil
	}

	// Get tools directly from MCP server without caching
	tools := s.mcpServer.ListTools()
	var openAITools []OpenAITool

	for _, tool := range tools {
		// Safely convert InputSchema to map[string]any
		var parameters map[string]any
		if tool.InputSchema != nil {
			if params, ok := tool.InputSchema.(map[string]any); ok {
				parameters = params
			} else {
				log.Warn().Str("user_id", user.Id).Str("tool_name", tool.Name).Msg("getMCPTools: Tool InputSchema is not a map, using empty parameters")
				parameters = make(map[string]any)
			}
		} else {
			parameters = make(map[string]any)
		}

		openAITools = append(openAITools, OpenAITool{
			Type: "function",
			Function: OpenAIToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  parameters,
			},
		})
	}

	return openAITools, nil
}

func (s *Service) executeMCPTool(ctx context.Context, toolCall ToolCall, user *model.User) (string, error) {
	if s.mcpServer == nil {
		log.Error().Str("user_id", user.Id).Str("tool_name", toolCall.Function.Name).Msg("executeMCPTool: MCP server not configured")
		return "", fmt.Errorf("MCP server is not configured")
	}

	log.Debug().Str("user_id", user.Id).Str("tool_name", toolCall.Function.Name).Msg("executeMCPTool: Executing MCP tool")

	// Add user to context for MCP server
	ctxWithUser := context.WithValue(ctx, "user", user)

	// Call tool directly using MCP server's CallTool method
	response, err := s.mcpServer.CallTool(ctxWithUser, toolCall.Function.Name, toolCall.Function.Arguments)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.Id).Str("tool_name", toolCall.Function.Name).Msg("executeMCPTool: MCP tool call failed")
		return "", fmt.Errorf("MCP tool call failed: %v", err)
	}

	// Extract text content from response
	if len(response.Content) > 0 && response.Content[0].Type == "text" {
		return response.Content[0].Text, nil
	}

	log.Warn().Str("user_id", user.Id).Str("tool_name", toolCall.Function.Name).Msg("executeMCPTool: Tool executed but returned no text content")
	return "Tool executed successfully", nil
}

func (s *Service) HandleChatStream(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	if user == nil {
		log.Error().Msg("HandleChatStream: User not found in context")
		rest.WriteResponse(http.StatusUnauthorized, w, r, map[string]string{
			"error": "User not found",
		})
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("HandleChatStream: Failed to decode request body")
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	if len(req.Messages) == 0 {
		log.Error().Msg("HandleChatStream: No messages provided in request")
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "No messages provided",
		})
		return
	}

	log.Debug().Str("user_id", user.Id).Int("message_count", len(req.Messages)).Msg("HandleChatStream: Starting chat stream")

	err := s.streamChat(r.Context(), req.Messages, user, w, r)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.Id).Msg("HandleChatStream: Stream chat failed")
		// Try to send error via SSE if possible, otherwise fall back to HTTP error
		s.sendErrorToStream(w, r, err)
		return
	}
}

// sendErrorToStream attempts to send an error via SSE, falls back to HTTP error
func (s *Service) sendErrorToStream(w http.ResponseWriter, r *http.Request, err error) {
	// Try to create stream writer and send error
	sseWriter := rest.NewStreamWriter(w, r)
	defer sseWriter.Close()

	writeErr := sseWriter.WriteChunk(SSEEvent{
		Type: "error",
		Data: map[string]string{"error": s.formatUserFriendlyError(err)},
	})

	if writeErr != nil {
		// Fall back to HTTP error if SSE fails
		http.Error(w, s.formatUserFriendlyError(err), http.StatusInternalServerError)
	}
}

// isRetryableError determines if an error is worth retrying
func (s *Service) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Retry on timeout, connection issues, and 5xx errors
	retryablePatterns := []string{
		"context deadline exceeded",
		"timeout",
		"connection refused",
		"connection reset",
		"temporary failure",
		"500",
		"502",
		"503",
		"504",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// formatUserFriendlyError converts technical errors to user-friendly messages
func (s *Service) formatUserFriendlyError(err error) string {
	if err == nil {
		return "An unknown error occurred"
	}

	errStr := err.Error()

	// Check for common error patterns and provide user-friendly messages
	if strings.Contains(errStr, "context deadline exceeded") || strings.Contains(errStr, "timeout") {
		return "The AI service is taking too long to respond. Please try again."
	}

	if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "no such host") {
		return "Unable to connect to the AI service. Please check your connection and try again."
	}

	if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "429") {
		return "The AI service is currently busy. Please wait a moment and try again."
	}

	if strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") {
		return "AI service authentication failed. Please contact your administrator."
	}

	if strings.Contains(errStr, "400") || strings.Contains(errStr, "bad request") {
		return "Invalid request sent to AI service. Please try rephrasing your message."
	}

	if strings.Contains(errStr, "500") || strings.Contains(errStr, "internal server error") {
		return "The AI service encountered an internal error. Please try again later."
	}

	if strings.Contains(errStr, "MCP") {
		return errStr
	}

	// For any other error, provide a generic message
	return "The AI service is currently unavailable. Please try again in a moment."
}

// addToContentBuffer adds content to the buffer and flushes if needed
func (s *Service) addToContentBuffer(content string, streamState *streamState, sseWriter *rest.StreamWriter) error {
	if content == "" {
		return nil
	}

	streamState.contentBuffer.WriteString(content)

	// Flush if buffer is large enough or maximum time has passed
	if streamState.contentBuffer.Len() >= CONTENT_BATCH_SIZE || (streamState.contentBuffer.Len() > 0 && time.Since(streamState.lastFlushTime) >= CONTENT_BATCH_TIME) {
		return s.flushContentBuffer(streamState, sseWriter)
	}

	return nil
}

// flushContentBuffer sends accumulated content to the client
func (s *Service) flushContentBuffer(streamState *streamState, sseWriter *rest.StreamWriter) error {
	if streamState.contentBuffer.Len() == 0 {
		return nil
	}

	content := streamState.contentBuffer.String()
	streamState.contentBuffer.Reset()
	streamState.lastFlushTime = time.Now()

	return sseWriter.WriteChunk(SSEEvent{
		Type: "content",
		Data: content,
	})
}

// GetInternalSystemPrompt returns the embedded system prompt (for scaffold command)
func GetInternalSystemPrompt() string {
	return defaultSystemPrompt
}
