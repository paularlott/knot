package chat

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/mcp"
	ai "github.com/paularlott/mcp/ai"
	mcpopenai "github.com/paularlott/mcp/ai/openai"

	"github.com/paularlott/knot/internal/log"
)

type Service struct {
	config config.ChatConfig
	client ai.Client
}

func NewService(cfg config.ChatConfig, mcpServer *mcp.Server) (*Service, error) {
	aiClient, err := ai.NewClient(ai.Config{
		Config: mcpopenai.Config{
			APIKey:      cfg.APIKey,
			BaseURL:     cfg.BaseURL,
			LocalServer: mcpServer,
		},
		Provider: ai.Provider(cfg.Provider),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AI client: %w", err)
	}

	chatService := &Service{
		config: cfg,
		client: aiClient,
	}

	return chatService, nil
}

func (s *Service) GetAIClient() ai.Client {
	return s.client
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
	req := mcpopenai.ChatCompletionRequest{
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
	response, err := s.client.ChatCompletion(ctx, req)
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

func (s *Service) convertMessagesToOpenAI(messages []ChatMessage) []mcpopenai.Message {
	openAIMessages := make([]mcpopenai.Message, 0, len(messages)+1)

	// Check if first message is a system message
	hasSystemMessage := len(messages) > 0 && messages[0].Role == "system"

	// Add system prompt only if no system message is present
	if !hasSystemMessage && s.config.SystemPrompt != "" {
		systemMessage := mcpopenai.Message{
			Role: "system",
		}
		systemMessage.SetContentAsString(s.config.SystemPrompt)
		openAIMessages = append(openAIMessages, systemMessage)
	}

	// Convert all messages (including any system messages from input)
	for _, msg := range messages {
		openAIMessage := mcpopenai.Message{
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
