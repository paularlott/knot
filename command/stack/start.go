package command_stack

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var StartCmd = &cli.Command{
	Name:        "start",
	Usage:       "Start a stack",
	Description: "Start all spaces in the named stack.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "stack",
			Usage:    "The name of the stack to start",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		stackName := cmd.GetStringArg("stack")
		fmt.Println("Starting stack: ", stackName)

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		_, err = client.StartStack(context.Background(), stackName)
		if err != nil {
			return fmt.Errorf("Error starting stack: %w", err)
		}

		fmt.Println("Stack started: ", stackName)
		return nil
	},
}
