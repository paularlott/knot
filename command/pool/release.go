package command_pool

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var ReleaseCmd = &cli.Command{
	Name:        "release",
	Usage:       "Release a leased pool member",
	Description: "End the lease held on a pool member. The member returns to the pool once any in-flight work has finished (normally within one sweep, ~15s). With --destroy the member is deleted instead and a fresh replacement is created, so the next acquire gets a clean space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The held member's name or space id — the same identifier acquire returned",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "destroy",
			Usage: "Destroy the member instead of returning it — a fresh replacement is created, so the next acquire gets a clean space.",
		},
		leaseJSONFlag(),
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceArg := cmd.GetStringArg("space")
		destroy := cmd.GetBool("destroy")

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		lease, code, err := client.ReleaseSpaceLease(context.Background(), spaceArg, destroy)
		if err != nil {
			return handleLeaseError(cmd, code, err, "")
		}

		if cmd.GetBool("json") {
			return printLeaseJSON(lease)
		}

		if destroy {
			fmt.Printf("Member %s destroyed; a fresh replacement is starting in pool %q.\n", spaceArg, lease.PoolName)
		} else {
			fmt.Printf("Member %s released; it returns to pool %q after in-flight work drains.\n", spaceArg, lease.PoolName)
		}
		return nil
	},
}
