package command_pool

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var DeleteCmd = &cli.Command{
	Name:        "delete",
	Usage:       "Delete a pool",
	Description: "Delete a stopped pool and all its member spaces.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "pool",
			Usage:    "The name or ID of the pool to delete",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "yes",
			Aliases: []string{"y"},
			Usage:   "Skip confirmation prompt.",
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		poolName := cmd.GetStringArg("pool")

		if !cmd.GetBool("yes") {
			fmt.Printf("Delete pool %q and all its spaces? [y/N] ", poolName)
			var resp string
			fmt.Scanln(&resp)
			if resp != "y" && resp != "Y" && resp != "yes" {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		code, err := client.DeletePool(context.Background(), poolName)
		if err != nil {
			return fmt.Errorf("Error deleting pool: %w (code %d)", err, code)
		}

		fmt.Fprintln(os.Stderr, "Pool deleted:", poolName)
		return nil
	},
}
