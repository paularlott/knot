package scripts

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
)

var deleteCmd = &cli.Command{
	Name:        "delete",
	Usage:       "Delete a script",
	Description: "Delete a script by name.",
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
		script, err := client.GetScriptByName(ctx, args[0])
		if err != nil {
			return fmt.Errorf("error getting script: %w", err)
		}

		err = client.DeleteScript(ctx, script.Id)
		if err != nil {
			return fmt.Errorf("error deleting script: %w", err)
		}

		fmt.Printf("Script %s deleted\n", args[0])
		return nil
	},
}
