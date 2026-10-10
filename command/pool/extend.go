package command_pool

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var ExtendCmd = &cli.Command{
	Name:        "extend",
	Usage:       "Extend the lease on a pool member",
	Description: "Renew a lease: the new deadline becomes now + the given duration (or never, on unlimited pools). Bounded by the pool's maximum extension count.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The held member's name or space id — the same identifier acquire returned",
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
		spaceArg := cmd.GetStringArg("space")

		duration, err := parseLeaseDuration(cmd.GetString("time"))
		if err != nil {
			return err
		}

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}

		lease, code, err := client.ExtendSpaceLease(context.Background(), spaceArg, &apiclient.PoolLeaseExtendRequest{
			DurationSeconds: duration,
		})
		if err != nil {
			return handleLeaseError(cmd, code, err, fmt.Sprintf("lease on %s cannot be extended (limit reached or already ended)", spaceArg))
		}

		if cmd.GetBool("json") {
			return printLeaseJSON(lease)
		}
		printLease(lease, "extended")
		return nil
	},
}
