package command_pool

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var ListCmd = &cli.Command{
	Name:        "list",
	Usage:       "List your pools",
	Description: "Lists all pools you own with their status and member count.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		pools, code, err := client.GetPools(context.Background())
		if err != nil {
			if code == 404 {
				fmt.Println("No pools found.")
				return nil
			}
			return fmt.Errorf("Error listing pools: %w", err)
		}

		if len(pools.Pools) == 0 {
			fmt.Println("No pools found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSTATE\tALIVE/DESIRED\tTEMPLATE")
		for _, p := range pools.Pools {
			state := "stopped"
			if p.Active {
				state = "active"
			}
			fmt.Fprintf(w, "%s\t%s\t%d/%d\t%s\n", p.Name, state, p.AliveMembers, p.DesiredCount, p.TemplateId)
		}
		w.Flush()
		return nil
	},
}
