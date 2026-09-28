package command_pool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var ReleaseCmd = &cli.Command{
	Name:        "release",
	Usage:       "Release a held pool lease",
	Description: "End a lease early. The member returns to the pool once any in-flight work has finished (normally within one sweep, ~15s).",
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
		leaseJSONFlag(),
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		poolName := cmd.GetStringArg("pool")
		leaseArg := cmd.GetStringArg("lease")

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

		lease, code, err := client.ReleasePoolLease(context.Background(), poolName, leaseId)
		if err != nil {
			return handleLeaseError(cmd, code, err, "")
		}

		if cmd.GetBool("json") {
			return printLeaseJSON(lease)
		}

		fmt.Printf("Lease %s released; the member returns to pool %q after in-flight work drains.\n", leaseArg, poolName)
		return nil
	},
}
