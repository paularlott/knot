package space

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var SpaceShutdownCmd = &cli.Command{
	Name:        "shutdown",
	Usage:       "Shutdown this space",
	Description: "Shutdown the space calling this command.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		err := agentlink.SendWithResponseMsg(agentlink.CommandSpaceStop, nil, nil)
		if err != nil {
			return fmt.Errorf("error requesting space shutdown: %w", err)
		}

		fmt.Println("Shutdown requested.")
		return nil
	},
}
