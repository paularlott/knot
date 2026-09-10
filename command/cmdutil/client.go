package cmdutil

import (
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"
)

// ExplicitServerAddr returns the server the user explicitly targeted via a
// --server and --token pair (flag or env), or via an --alias that resolves in
// the config file. It returns nil when neither is given, so inside a space
// callers can fall back to the server that owns the space.
//
// Note the both-non-empty rule for the flags: spaces always have KNOT_SERVER
// set to the owning server's URL, so a token alone must never count as an
// explicit target. An --alias is only honoured when actually given on the
// command line — the "default" alias is never consulted implicitly, so a
// config file carried into a space cannot silently redirect tunnels.
func ExplicitServerAddr(cmd *cli.Command) *config.ServerAddr {
	if server, token := cmd.GetString("server"), cmd.GetString("token"); server != "" && token != "" {
		return config.NewServerAddr(server, token)
	}
	if cmd.HasFlag("alias") {
		if cfg, ok := config.LookupServerAddr(cmd.GetString("alias"), cmd); ok {
			return cfg
		}
	}
	return nil
}

func GetServerAddr(cmd *cli.Command) *config.ServerAddr {
	if agentlink.IsAgentRunning() {
		// In a space: an explicit --server/--token or --alias tunnels via
		// that server from this process; otherwise the agentlink socket
		// carries the connection to the server that owns the space.
		if cfg := ExplicitServerAddr(cmd); cfg != nil {
			return cfg
		}

		server, token, err := agentlink.GetConnectionInfo()
		if err != nil {
			fmt.Printf("Error: failed to get agent connection info: %v\n", err)
			return nil
		}

		return config.NewServerAddr(server, token)
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
