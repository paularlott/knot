package cmdutil

import (
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"
)

// GetClient returns an API client that works in both desktop and agent contexts.
// It checks if an agent is running and if so, uses the agent's connection info.
// Otherwise, it falls back to desktop configuration.
func GetClient(cmd *cli.Command) (*apiclient.ApiClient, error) {
	// Check if agent is running
	if agentlink.IsAgentRunning() {
		// Get connection info from agent
		server, token, err := agentlink.GetConnectionInfo()
		if err != nil {
			return nil, fmt.Errorf("failed to get agent connection info: %w", err)
		}

		// Create client with agent credentials
		client, err := apiclient.NewClient(server, token, true)
		if err != nil {
			return nil, fmt.Errorf("failed to create agent API client: %w", err)
		}

		return client, nil
	}

	// Fall back to desktop configuration
	alias := cmd.GetString("alias")
	cfg := config.GetServerAddr(alias, cmd)

	if cfg.HttpServer == "" {
		return nil, fmt.Errorf("no server configured")
	}

	client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return client, nil
}
