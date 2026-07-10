package command_tunnel

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var StopTunnelCmd = &cli.Command{
	Name:        "stop",
	Usage:       "Stop a daemon tunnel",
	Description: `Stop a tunnel running in the knot agent by its name.`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "The name of the tunnel to stop",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		name := cmd.GetStringArg("name")

		request := agentlink.StopTunnelRequest{
			Name: name,
		}

		var response agentlink.RunCommandResponse
		if err := agentlink.SendWithResponseMsg(agentlink.CommandStopTunnel, &request, &response); err != nil {
			return fmt.Errorf("error stopping tunnel: %w", err)
		}

		if !response.Success {
			return fmt.Errorf("%s", response.Error)
		}

		fmt.Printf("Tunnel %s stopped.\n", name)
		return nil
	},
}
