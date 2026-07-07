package chat

import (
	"fmt"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/mcp"
	ai "github.com/paularlott/mcp/ai"
	mcpopenai "github.com/paularlott/mcp/ai/openai"
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

	return &Service{
		config: cfg,
		client: aiClient,
	}, nil
}

func (s *Service) GetAIClient() ai.Client {
	return s.client
}
