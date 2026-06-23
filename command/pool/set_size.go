package command_pool

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var SetSizeCmd = &cli.Command{
	Name:        "set-size",
	Usage:       "Set the desired space count for a pool",
	Description: "Update the target number of spaces for a pool. The sweep loop handles creating or removing members.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "pool",
			Usage:    "The name or ID of the pool",
			Required: true,
		},
		&cli.IntArg{
			Name:     "count",
			Usage:    "The desired number of spaces (minimum 1)",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		poolName := cmd.GetStringArg("pool")
		desired := cmd.GetIntArg("count")

		if desired < 1 {
			return fmt.Errorf("count must be at least 1")
		}

		fmt.Printf("Setting pool %q size to %d...\n", poolName, desired)

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		code, err := client.SetPoolSize(context.Background(), poolName, desired)
		if err != nil {
			return fmt.Errorf("Error setting pool size: %w (code %d)", err, code)
		}

		fmt.Printf("Pool %q size set to %d.\n", poolName, desired)
		return nil
	},
}
