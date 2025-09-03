package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/openai"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/mcp"
	"github.com/rs/zerolog/log"
)

const (
	CONTENT_BATCH_SIZE = 150                    // characters - smaller for more responsive streaming
	CONTENT_BATCH_TIME = 100 * time.Millisecond // shorter time for more responsive streaming
)

type Service struct {
	config       config.ChatConfig
	openaiClient *openai.Client
}

type streamState struct {
	inThinking    bool
	inReasoning   bool
	contentBuffer strings.Builder // Content batching
	lastFlushTime time.Time
}

func NewService(config config.ChatConfig, mcpServer *mcp.Server) (*Service, error) {
	// Create OpenAI client
	openaiConfig := openai.Config{
		APIKey:  config.OpenAIAPIKey,
		BaseURL: config.OpenAIBaseURL,
		Timeout: 5 * time.Minute,
	}

	// Convert mcp.Server to openai.MCPServer interface
	var mcpServerInterface openai.MCPServer
	if mcpServer != nil {
		mcpServerInterface = mcpServer
	}

	openaiClient, err := openai.New(openaiConfig, mcpServerInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI client: %w", err)
	}

	chatService := &Service{
		config:       config,
		openaiClient: openaiClient,
	}

	return chatService, nil
}

func (s *Service) GetOpenAIClient() *openai.Client {
	return s.openaiClient
}

func (s *Service) streamChat(ctx context.Context, messages []ChatMessage, user *model.User, w http.ResponseWriter, r *http.Request) error {
	if len(messages) == 0 {
		log.Warn().Str("user_id", user.Id).Msg("streamChat: No messages provided")
		return nil
	}

	// Check if client disconnected before starting
	select {
	case <-ctx.Done():
		log.Trace().Str("user_id", user.Id).Msg("streamChat: Client disconnected before processing")
		return ctx.Err()
	default:
	}

	sseWriter := rest.NewStreamWriter(w, r)
	defer sseWriter.Close()

	// Initialize stream state for content batching
	streamState := &streamState{
		lastFlushTime: time.Now(),
	}

	// Convert messages to OpenAI format
	openAIMessages := s.convertMessagesToOpenAI(messages)

	// Create request
	req := openai.ChatCompletionRequest{
		Model:           s.config.Model,
		Messages:        openAIMessages,
		MaxTokens:       s.config.MaxTokens,
		Temperature:     s.config.Temperature,
		ReasoningEffort: s.config.ReasoningEffort,
	}

	log.Debug().Str("user_id", user.Id).Str("model", s.config.Model).Msg("streamChat: Starting chat completion with tools")

	// Add user to context for MCP server
	chatCtx := context.WithValue(ctx, "user", user)

	// Add the tool handler to the context
	toolHandler := NewWebChatToolHandler(sseWriter)
	chatCtx = openai.WithToolHandler(chatCtx, toolHandler)

	// Start streaming
	stream := s.openaiClient.StreamChatCompletion(chatCtx, req)
	for stream.Next() {
		response := stream.Current()

		// Process OpenAI response
		if len(response.Choices) > 0 {
			choice := response.Choices[0]

			if choice.Delta.Content != "" {
				if err := s.handleContentStream(choice.Delta.Content, streamState, sseWriter); err != nil {
					return err
				}
			}

			if choice.Delta.ReasoningContent != "" {
				if err := s.handleReasoningStream(choice.Delta.ReasoningContent, streamState, sseWriter); err != nil {
					return err
				}
			}
		}
	}

	// Check for errors after the loop - much cleaner!
	if err := stream.Err(); err != nil {
		log.Error().Err(err).Str("user_id", user.Id).Msg("streamChat: Chat completion with tools failed")
		sseWriter.WriteChunk(SSEEvent{
			Type: "error",
			Data: map[string]string{"error": s.formatUserFriendlyError(err)},
		})
		return err
	}

	// Stream completed successfully
	return s.handleDoneStream(streamState, sseWriter)
}

func (s *Service) convertMessagesToOpenAI(messages []ChatMessage) []openai.Message {
	openAIMessages := make([]openai.Message, 0, len(messages)+1)

	// Add system prompt (always first)
	if s.config.SystemPrompt != "" {
		systemMessage := openai.Message{
			Role: "system",
		}
		systemMessage.SetContentAsString(s.config.SystemPrompt)
		openAIMessages = append(openAIMessages, systemMessage)
	}

	// Convert chat messages, skipping any existing system messages from history
	for _, msg := range messages {
		if msg.Role != "system" {
			openAIMessage := openai.Message{
				Role:       msg.Role,
				ToolCalls:  msg.ToolCalls,
				ToolCallID: msg.ToolCallID,
			}
			openAIMessage.SetContentAsString(msg.Content)
			openAIMessages = append(openAIMessages, openAIMessage)
		}
	}

	return openAIMessages
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

	s.streamChat(r.Context(), req.Messages, user, w, r)
}

// formatUserFriendlyError converts technical errors to user-friendly messages
func (s *Service) formatUserFriendlyError(err error) string {
	if err == nil {
		return "An unknown error occurred"
	}

	originalErr := err.Error()
	errStr := strings.ToLower(originalErr)

	// Check for common error patterns and provide user-friendly messages
	switch {
	case strings.Contains(errStr, "context deadline exceeded") || strings.Contains(errStr, "timeout"):
		return "The AI service is taking too long to respond. Please try again."
	case strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "no such host"):
		return "Unable to connect to the AI service. Please check your connection and try again."
	case strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "429"):
		return "The AI service is currently busy. Please wait a moment and try again."
	case strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized"):
		return "AI service authentication failed. Please contact your administrator."
	case strings.Contains(errStr, "400") || strings.Contains(errStr, "bad request"):
		return "Invalid request sent to AI service. Please try rephrasing your message."
	case strings.Contains(errStr, "500") || strings.Contains(errStr, "internal server error"):
		return "The AI service encountered an internal error. Please try again later."
	case strings.Contains(originalErr, "MCP"): // Keep original case for MCP errors
		return originalErr
	default:
		return "The AI service is currently unavailable. Please try again in a moment."
	}
}

// addToContentBuffer adds content to the buffer and flushes if needed
func (s *Service) addToContentBuffer(content string, streamState *streamState, sseWriter *rest.StreamWriter) error {
	if content == "" {
		return nil
	}

	streamState.contentBuffer.WriteString(content)

	// Flush if buffer is large enough or maximum time has passed
	if streamState.contentBuffer.Len() >= CONTENT_BATCH_SIZE || time.Since(streamState.lastFlushTime) >= CONTENT_BATCH_TIME {
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

// Stream handler methods for different event types

func (s *Service) handleContentStream(content string, streamState *streamState, sseWriter *rest.StreamWriter) error {
	if streamState.inReasoning {
		// Wrap content in think tags for frontend processing
		if err := s.addToContentBuffer("</think>", streamState, sseWriter); err != nil {
			return err
		}
		streamState.inReasoning = false
	}

	if content == "<think>" {
		streamState.inThinking = true
	} else if content == "</think>" {
		streamState.inThinking = false
	}

	return s.addToContentBuffer(content, streamState, sseWriter)
}

func (s *Service) handleReasoningStream(content string, streamState *streamState, sseWriter *rest.StreamWriter) error {
	if !streamState.inReasoning {
		// Wrap reasoning content in think tags for frontend processing
		if err := s.addToContentBuffer("<think>", streamState, sseWriter); err != nil {
			return err
		}
		streamState.inReasoning = true
	}

	return s.addToContentBuffer(content, streamState, sseWriter)
}

func (s *Service) handleDoneStream(streamState *streamState, sseWriter *rest.StreamWriter) error {
	// Flush any remaining content
	if streamState.inReasoning {
		if err := s.addToContentBuffer("</think>", streamState, sseWriter); err != nil {
			return err
		}
		streamState.inReasoning = false
	}
	if err := s.flushContentBuffer(streamState, sseWriter); err != nil {
		return err
	}

	return sseWriter.WriteChunk(SSEEvent{
		Type: "done",
		Data: nil,
	})
}
