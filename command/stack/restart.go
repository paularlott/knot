package command_stack

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var RestartCmd = &cli.Command{
	Name:        "restart",
	Usage:       "Restart a stack",
	Description: "Restart all spaces in the named stack.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "stack",
			Usage:    "The name of the stack to restart",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		stackName := cmd.GetStringArg("stack")
		fmt.Println("Restarting stack: ", stackName)

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		_, err = client.RestartStack(context.Background(), stackName)
		if err != nil {
			return fmt.Errorf("Error restarting stack: %w", err)
		}

		fmt.Println("Stack restarted: ", stackName)
		return nil
	},
}
