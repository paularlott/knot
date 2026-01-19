package scripts

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
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

		// Separate scripts into user and global
		var userScripts, globalScripts []apiclient.ScriptInfo
		for _, script := range scripts.Scripts {
			if script.UserId != "" {
				userScripts = append(userScripts, script)
			} else {
				globalScripts = append(globalScripts, script)
			}
		}

		// Print user scripts first (if any)
		if len(userScripts) > 0 {
			fmt.Println("\nUser Scripts:")
			table := [][]string{
				{"NAME", "DESCRIPTION", "ACTIVE", "TYPE"},
			}
			for _, script := range userScripts {
				active := "No"
				if script.Active {
					active = "Yes"
				}
				table = append(table, []string{
					script.Name,
					script.Description,
					active,
					script.ScriptType,
				})
			}
			util.PrintTable(table)
		}

		// Print global scripts (if any)
		if len(globalScripts) > 0 {
			fmt.Println("\nGlobal Scripts:")
			table := [][]string{
				{"NAME", "DESCRIPTION", "ACTIVE", "TYPE"},
			}
			for _, script := range globalScripts {
				active := "No"
				if script.Active {
					active = "Yes"
				}
				table = append(table, []string{
					script.Name,
					script.Description,
					active,
					script.ScriptType,
				})
			}
			util.PrintTable(table)
		}

		return nil
	},
}
