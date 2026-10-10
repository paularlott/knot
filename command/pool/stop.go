package command_pool

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var StopCmd = &cli.Command{
	Name:        "stop",
	Usage:       "Stop a pool",
	Description: "Stop a running pool and all its member spaces without deleting them.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "pool",
			Usage:    "The name or ID of the pool to stop",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		poolName := cmd.GetStringArg("pool")
		fmt.Println("Stopping pool:", poolName)

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}

		_, err = client.StopPool(context.Background(), poolName)
		if err != nil {
			return fmt.Errorf("couldn't stop pool %q: %w", poolName, err)
		}

		fmt.Println("Pool stopped:", poolName)
		return nil
	},
}
