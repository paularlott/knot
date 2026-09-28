package command_pool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var LeasesCmd = &cli.Command{
	Name:        "leases",
	Usage:       "List a pool's leases",
	Description: "Lists the pool's exclusively held leases — active plus draining (ended, waiting for in-flight work to finish).",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "pool",
			Usage:    "The name or ID of the pool",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		leaseJSONFlag(),
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		poolName := cmd.GetStringArg("pool")

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		leases, code, err := client.GetPoolLeases(context.Background(), poolName)
		if err != nil {
			return handleLeaseError(cmd, code, err, "")
		}

		if cmd.GetBool("json") {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(leases)
		}

		if len(leases.Leases) == 0 {
			fmt.Printf("No leases held on pool %q.\n", poolName)
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "LEASE\tMEMBER\tSTATE\tEXPIRES\tEXTENSIONS\tHOLDER")
		for _, lease := range leases.Leases {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d/%s\t%s\n",
				lease.LeaseId,
				lease.SpaceName,
				lease.State,
				formatLeaseExpiry(lease.ExpiresAt),
				lease.ExtensionsUsed,
				formatMaxExtensions(lease.MaxExtensions),
				lease.Username,
			)
		}
		w.Flush()
		return nil
	},
}

func formatMaxExtensions(max int) string {
	if max < 0 {
		return "unlimited"
	}
	return fmt.Sprintf("%d", max)
}

// printLease renders one lease for acquire/extend output.
func printLease(lease *apiclient.LeaseInfo, verb string) {
	fmt.Printf("Lease %s %s on member %s (space id %s).\n", lease.LeaseId, verb, lease.SpaceName, lease.SpaceId)
	fmt.Printf("  Expires: %s\n", formatLeaseExpiry(lease.ExpiresAt))
	fmt.Printf("  Extensions used: %d (max %s)\n", lease.ExtensionsUsed, formatMaxExtensions(lease.MaxExtensions))
	fmt.Printf("  Use the member by its own name or pin method calls with space_id %s.\n", lease.SpaceId)
}
