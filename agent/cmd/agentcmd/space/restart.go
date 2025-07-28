package space

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var SpaceRestartCmd = &cli.Command{
	Name:        "restart",
	Usage:       "Restart this space",
	Description: "Restart the space calling this command.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		err := agentlink.SendWithResponseMsg(agentlink.CommandSpaceRestart, nil, nil)
		if err != nil {
			return fmt.Errorf("error requesting space restart: %w", err)
		}

		fmt.Println("Restart requested.")
		return nil
	},
}
