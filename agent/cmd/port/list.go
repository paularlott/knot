package port

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var ListPortForwardsCmd = &cli.Command{
	Name:        "list",
	Usage:       "List active port forwards",
	Description: "List all active port forwards in the current space.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		var response agentlink.ListPortForwardsResponse
		err := agentlink.SendWithResponseMsg(agentlink.CommandListPortForwards, nil, &response)
		if err != nil {
			return fmt.Errorf("error listing port forwards: %w", err)
		}

		if len(response.Forwards) == 0 {
			fmt.Println("No active port forwards.")
			return nil
		}

		fmt.Println("Active port forwards:")
		for _, fwd := range response.Forwards {
			fmt.Printf("  %d -> %s:%d\n", fwd.LocalPort, fwd.Space, fwd.RemotePort)
		}
		return nil
	},
}
