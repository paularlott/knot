package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var StopCmd = &cli.Command{
	Name:        "stop",
	Usage:       "Stop a space",
	Description: "Stop the named space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space to stop",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		fmt.Println("Stopping space: ", spaceName)

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		// Stop the space (API supports both name and ID)
		_, err = client.StopSpace(context.Background(), spaceName)
		if err != nil {
			return fmt.Errorf("Error stopping space: %w", err)
		}

		fmt.Println("Space stopped: ", spaceName)
		return nil
	},
}
