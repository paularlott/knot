package chat

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/openai"
	"github.com/paularlott/mcp"

	"github.com/paularlott/knot/internal/log"
)

type Service struct {
	config       config.ChatConfig
	openaiClient *openai.Client
}

func NewService(config config.ChatConfig, mcpServer *mcp.Server) (*Service, error) {
	// Create OpenAI client
	openaiConfig := openai.Config{
		APIKey:  config.OpenAIAPIKey,
		BaseURL: config.OpenAIBaseURL,
	}

	openaiClient, err := openai.New(openaiConfig, mcpServer)
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

// ChatCompletion performs a non-streaming chat completion
func (s *Service) ChatCompletion(ctx context.Context, messages []ChatMessage, user *model.User) (*ChatCompletionResponse, error) {
	logger := log.WithGroup("chat")

	if len(messages) == 0 {
		logger.Warn("No messages provided", "user_id", user.Id)
		return &ChatCompletionResponse{
			Content: "",
		}, nil
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

	logger.Debug("Starting chat completion with tools", "user_id", user.Id, "model", s.config.Model)

	// The MCPServerContext middleware has already set up the context with:
	// - MCP server
	// - Script tools provider
	// - Force on-demand mode
	// Use the context directly
	response, err := s.openaiClient.ChatCompletion(ctx, req)
	if err != nil {
		logger.Error("Chat completion failed", "error", err, "user_id", user.Id)
		return nil, err
	}

	// Extract content
	content := ""
	if len(response.Choices) > 0 {
		content = response.Choices[0].Message.GetContentAsString()
	}

	// Strip think tags for consistency with streaming
	content = stripThinkTags(content)

	return &ChatCompletionResponse{
		Content: content,
	}, nil
}

func (s *Service) convertMessagesToOpenAI(messages []ChatMessage) []openai.Message {
	openAIMessages := make([]openai.Message, 0, len(messages)+1)

	// Check if first message is a system message
	hasSystemMessage := len(messages) > 0 && messages[0].Role == "system"

	// Add system prompt only if no system message is present
	if !hasSystemMessage && s.config.SystemPrompt != "" {
		systemMessage := openai.Message{
			Role: "system",
		}
		systemMessage.SetContentAsString(s.config.SystemPrompt)
		openAIMessages = append(openAIMessages, systemMessage)
	}

	// Convert all messages (including any system messages from input)
	for _, msg := range messages {
		openAIMessage := openai.Message{
			Role:       msg.Role,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		openAIMessage.SetContentAsString(msg.Content)
		openAIMessages = append(openAIMessages, openAIMessage)
	}

	return openAIMessages
}

// stripThinkTags removes <think>...</think> tags from content to prevent LLM template errors
func stripThinkTags(content string) string {
	// Use regex to remove think tags and their content
	re := regexp.MustCompile(`(?s)<think>.*?</think>`)
	return strings.TrimSpace(re.ReplaceAllString(content, ""))
}
