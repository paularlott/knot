package command_pool

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var StartCmd = &cli.Command{
	Name:        "start",
	Usage:       "Start a pool",
	Description: "Start a stopped pool and all its member spaces.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "pool",
			Usage:    "The name or ID of the pool to start",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		poolName := cmd.GetStringArg("pool")
		fmt.Println("Starting pool:", poolName)

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		code, err := client.StartPool(context.Background(), poolName)
		if err != nil {
			return fmt.Errorf("Error starting pool: %w (code %d)", err, code)
		}

		fmt.Println("Pool started:", poolName)
		return nil
	},
}
