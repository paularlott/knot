package scripts

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
)

var showCmd = &cli.Command{
	Name:        "show",
	Usage:       "Show script details",
	Description: "Show details of a specific script.",
	MinArgs:     1,
	MaxArgs:     1,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		args := cmd.GetArgs()
		script, err := client.GetScriptDetailsByName(ctx, args[0])
		if err != nil {
			return fmt.Errorf("error getting script: %w", err)
		}

		fmt.Printf("Name: %s\n", script.Name)
		fmt.Printf("Description: %s\n", script.Description)
		fmt.Printf("Active: %t\n", script.Active)
		fmt.Printf("Type: %s\n", script.ScriptType)
		fmt.Printf("Timeout: %ds\n", script.Timeout)
		fmt.Printf("Groups: %v\n", script.Groups)
		if script.ScriptType == "tool" {
			fmt.Printf("MCP Keywords: %v\n", script.MCPKeywords)
		}
		fmt.Printf("\nContent:\n%s\n", script.Content)
		return nil
	},
}
