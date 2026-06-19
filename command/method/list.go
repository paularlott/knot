package command_method

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util"
)

var listCmd = &cli.Command{
	Name:    "list",
	Usage:   "List visible methods",
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}
		methods, err := client.GetMethods(ctx)
		if err != nil {
			return err
		}
		if methods.Count == 0 {
			fmt.Println("No methods found")
			return nil
		}
		table := [][]string{{"NAME", "DESCRIPTION", "SCOPE", "MCP"}}
		for _, method := range methods.Methods {
			mcp := "No"
			if method.MCPTool {
				mcp = "Yes"
			}
			table = append(table, []string{method.Name, method.Description, method.Scope, mcp})
		}
		util.PrintTable(table)
		return nil
	},
}
