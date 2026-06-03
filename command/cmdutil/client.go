package cmdutil

import (
	"fmt"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"
)

func GetServerAddr(cmd *cli.Command) *config.ServerAddr {
	if agentlink.IsAgentRunning() {
		server, token, err := agentlink.GetConnectionInfo()
		if err != nil {
			fmt.Printf("Error: failed to get agent connection info: %v\n", err)
			return nil
		}

		httpServer := server
		if !strings.HasPrefix(httpServer, "http://") && !strings.HasPrefix(httpServer, "https://") {
			httpServer = "https://" + httpServer
		}
		httpServer = strings.TrimSuffix(httpServer, "/")

		return &config.ServerAddr{
			HttpServer: httpServer,
			WsServer:   "ws" + httpServer[4:],
			ApiToken:   token,
		}
	}

	alias := cmd.GetString("alias")
	return config.GetServerAddr(alias, cmd)
}

func GetClient(cmd *cli.Command) (*apiclient.ApiClient, error) {
	if agentlink.IsAgentRunning() {
		server, token, err := agentlink.GetConnectionInfo()
		if err != nil {
			return nil, fmt.Errorf("failed to get agent connection info: %w", err)
		}

		client, err := apiclient.NewClient(server, token, true)
		if err != nil {
			return nil, fmt.Errorf("failed to create agent API client: %w", err)
		}

		return client, nil
	}

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
