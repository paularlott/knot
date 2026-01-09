package scripts

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util"
)

var listCmd = &cli.Command{
	Name:    "list",
	Usage:   "List scripts",
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		scripts, err := client.GetScripts(ctx)
		if err != nil {
			return fmt.Errorf("error getting scripts: %w", err)
		}

		if scripts.Count == 0 {
			fmt.Println("No scripts found")
			return nil
		}

		table := [][]string{
			{"NAME", "DESCRIPTION", "ACTIVE", "TYPE", "TIMEOUT"},
		}

		for _, script := range scripts.Scripts {
			active := "No"
			if script.Active {
				active = "Yes"
			}

			table = append(table, []string{
				script.Name,
				script.Description,
				active,
				script.ScriptType,
				fmt.Sprintf("%ds", script.Timeout),
			})
		}

		util.PrintTable(table)
		return nil
	},
}
