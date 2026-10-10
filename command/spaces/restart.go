package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var RestartCmd = &cli.Command{
	Name:        "restart",
	Usage:       "Restart a space",
	Description: "Restart the named space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space to restart",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		fmt.Println("Restarting space: ", spaceName)

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}

		// Restart the space (API supports both name and ID)
		_, err = client.RestartSpace(context.Background(), spaceName)
		if err != nil {
			return fmt.Errorf("couldn't restart space %q: %w", spaceName, err)
		}

		fmt.Println("Space restarting: ", spaceName)
		return nil
	},
}
