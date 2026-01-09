package command

import (
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"
)

// GetClient returns an API client that works in both desktop and agent contexts.
// It first checks if an agent is running and uses the agent's connection info.
// If no agent is running, it falls back to desktop configuration.
func GetClient(cmd *cli.Command) (*apiclient.ApiClient, error) {
	// Check if running in agent context
	if agentlink.IsAgentRunning() {
		// Get connection info from agent
		server, token, err := agentlink.GetConnectionInfo()
		if err != nil {
			return nil, fmt.Errorf("failed to get agent connection info: %w", err)
		}

		// Create client with agent credentials
		client, err := apiclient.NewClient(server, token, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return nil, fmt.Errorf("failed to create agent API client: %w", err)
		}

		return client, nil
	}

	// Fall back to desktop configuration
	alias := cmd.GetString("alias")
	cfg := config.GetServerAddr(alias, cmd)

	client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
	if err != nil {
		return nil, fmt.Errorf("failed to create desktop API client: %w", err)
	}

	return client, nil
}
