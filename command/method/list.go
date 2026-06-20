package command_method

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/methods"
	"github.com/paularlott/knot/internal/util"
)

var listCmd = &cli.Command{
	Name:      "list",
	Usage:     "List visible methods or show details for a specific method",
	MaxArgs:   1,
	Arguments: []cli.Argument{&cli.StringArg{Name: "method", Usage: "Optional method name to show full details (params/result schemas, owner, scope, etc.)", Required: false}},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}
		result, err := client.GetMethods(ctx)
		if err != nil {
			return err
		}
		if result.Count == 0 {
			fmt.Println("No methods found")
			return nil
		}

		// If a method name is given, show details for that method only.
		methodName := cmd.GetStringArg("method")
		if methodName != "" {
			return showMethodDetail(result.Methods, methodName)
		}

		// No argument — table view (existing behaviour).
		table := [][]string{{"NAME", "DESCRIPTION", "SCOPE", "MCP", "PROVIDERS"}}
		for _, method := range result.Methods {
			mcp := "No"
			if method.MCPTool {
				mcp = "Yes"
			}
			providers := fmt.Sprintf("%d", method.ProviderCount)
			if method.ProviderCount > 1 {
				providers = fmt.Sprintf("%d spaces", method.ProviderCount)
			}
			table = append(table, []string{method.Name, method.Description, method.Scope, mcp, providers})
		}
		util.PrintTable(table)
		return nil
	},
}

func showMethodDetail(methods []methods.MethodInfo, name string) error {
	for _, m := range methods {
		if m.Name != name {
			continue
		}

		fmt.Printf("Name:          %s\n", m.Name)
		fmt.Printf("Local Name:    %s\n", m.LocalName)
		fmt.Printf("Description:   %s\n", m.Description)
		fmt.Printf("Owner:         %s\n", m.Owner)
		fmt.Printf("Scope:         %s\n", m.Scope)
		if len(m.Groups) > 0 {
			fmt.Printf("Groups:        %v\n", m.Groups)
		}
		fmt.Printf("MCP Tool:      %s\n", boolStr(m.MCPTool, "Yes", "No"))
		if m.ProviderCount > 1 {
			fmt.Printf("Providers:     %d spaces\n", m.ProviderCount)
		}
		if len(m.Keywords) > 0 {
			fmt.Printf("Keywords:      %v\n", m.Keywords)
		}

		fmt.Println()
		fmt.Println("Params Schema:")
		printSchema(m.ParamsSchema)

		fmt.Println()
		fmt.Println("Result Schema:")
		printSchema(m.ResultSchema)

		return nil
	}

	return fmt.Errorf("method %q not found or not visible to you", name)
}

func printSchema(schema map[string]any) {
	if len(schema) == 0 {
		fmt.Println("  (none)")
		return
	}
	data, err := json.MarshalIndent(schema, "  ", "  ")
	if err != nil {
		fmt.Printf("  %v\n", schema)
		return
	}
	fmt.Printf("  %s\n", string(data))
}

func boolStr(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}
