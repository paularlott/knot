package command_pool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var ExtendCmd = &cli.Command{
	Name:        "extend",
	Usage:       "Extend a held pool lease",
	Description: "Renew a lease: the new deadline becomes now + the given duration (or never, on unlimited pools). Bounded by the pool's maximum extension count.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "pool",
			Usage:    "The name or ID of the pool",
			Required: true,
		},
		&cli.StringArg{
			Name:     "lease",
			Usage:    "The lease id from acquire, or the held member's name / space id",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "time",
			Usage: "How long to hold the member from now, e.g. 90s, 5m, 2h; 'none' for no expiry (unlimited pools). Defaults to the pool's configured maximum.",
		},
		leaseJSONFlag(),
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		poolName := cmd.GetStringArg("pool")
		leaseArg := cmd.GetStringArg("lease")

		duration, err := parseLeaseDuration(cmd.GetString("time"))
		if err != nil {
			return err
		}

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		leaseId, err := resolveLeaseArg(context.Background(), client, poolName, leaseArg)
		if err != nil {
			if cmd.GetBool("json") {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(leaseErrorJSON{State: "error", Error: err.Error()})
			}
			return err
		}

		lease, code, err := client.ExtendPoolLease(context.Background(), poolName, leaseId, &apiclient.PoolLeaseExtendRequest{
			DurationSeconds: duration,
		})
		if err != nil {
			return handleLeaseError(cmd, code, err, fmt.Sprintf("lease %s cannot be extended (limit reached or already ended)", leaseArg))
		}

		if cmd.GetBool("json") {
			return printLeaseJSON(lease)
		}
		printLease(lease, "extended")
		return nil
	},
}
