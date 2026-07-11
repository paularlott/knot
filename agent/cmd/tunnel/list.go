package command_tunnel

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var ListTunnelCmd = &cli.Command{
	Name:        "list",
	Usage:       "List daemon tunnels",
	Description: `List all tunnels running in the knot agent.`,
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		var response agentlink.ListTunnelsResponse
		if err := agentlink.SendWithResponseMsg(agentlink.CommandListTunnels, nil, &response); err != nil {
			return fmt.Errorf("error listing tunnels: %w", err)
		}

		if len(response.Tunnels) == 0 {
			fmt.Println("No active tunnels.")
			return nil
		}

		fmt.Println("Active tunnels:")
		for _, t := range response.Tunnels {
			fmt.Printf("  %s  %d  %s  %s\n", t.Name, t.Port, t.Protocol, t.URL)
		}
		return nil
	},
}
