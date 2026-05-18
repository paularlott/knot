package command_stack

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var StopCmd = &cli.Command{
	Name:        "stop",
	Usage:       "Stop a stack",
	Description: "Stop all spaces in the named stack.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "stack",
			Usage:    "The name of the stack to stop",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		stackName := cmd.GetStringArg("stack")
		fmt.Println("Stopping stack: ", stackName)

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		_, err = client.StopStack(context.Background(), stackName)
		if err != nil {
			return fmt.Errorf("Error stopping stack: %w", err)
		}

		fmt.Println("Stack stopped: ", stackName)
		return nil
	},
}
