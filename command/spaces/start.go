package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var StartCmd = &cli.Command{
	Name:        "start",
	Usage:       "Start a space",
	Description: "Start the named space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space to start",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		fmt.Println("Starting space: ", spaceName)

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}

		// Start the space (API supports both name and ID)
		code, err := client.StartSpace(context.Background(), spaceName)
		if err != nil {
			if code == 503 {
				return fmt.Errorf("couldn't start space %q: it is outside its schedule", spaceName)
			} else if code == 507 {
				return fmt.Errorf("couldn't start space %q: it would exceed your resource quota; stop another space first", spaceName)
			} else {
				return fmt.Errorf("couldn't start space %q: %w", spaceName, err)
			}
		}

		fmt.Println("Space started: ", spaceName)
		return nil
	},
}
