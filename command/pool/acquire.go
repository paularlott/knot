package command_pool

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var AcquireCmd = &cli.Command{
	Name:        "acquire",
	Usage:       "Acquire a pool member exclusively",
	Description: "Check out one member of a pool for exclusive use until the lease expires (or is released). Method calls and pool-name port routing skip the member while held; reach it by its own space name or by pinning calls with its space id.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "pool",
			Usage:    "The name or ID of the pool",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "time",
			Usage: "How long to hold the member, e.g. 90s, 5m, 2h; 'none' for a lease that never expires (unlimited pools). Defaults to the pool's configured maximum.",
		},
		&cli.StringFlag{
			Name:  "wait",
			Usage: "How long to wait for a free member if all are leased, e.g. 30s, 2m (max 5m). Defaults to failing immediately.",
		},
		leaseJSONFlag(),
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		poolName := cmd.GetStringArg("pool")

		duration, err := parseLeaseDuration(cmd.GetString("time"))
		if err != nil {
			return err
		}
		wait := 0
		if value := cmd.GetString("wait"); value != "" {
			wait, err = parseLeaseDuration(value)
			if err != nil {
				return fmt.Errorf("invalid --wait: %w", err)
			}
		}

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		if !cmd.GetBool("json") {
			fmt.Printf("Acquiring a member of pool %q...\n", poolName)
		}
		lease, code, err := client.AcquirePoolLease(context.Background(), poolName, &apiclient.PoolLeaseAcquireRequest{
			DurationSeconds: duration,
			WaitSeconds:     wait,
		})
		if err != nil {
			return handleLeaseError(cmd, code, err, fmt.Sprintf("no free member available in pool %q", poolName))
		}

		if cmd.GetBool("json") {
			return printLeaseJSON(lease)
		}
		printLease(lease, "acquired")
		return nil
	},
}
