package scripts

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
)

var runCmd = &cli.Command{
	Name:        "run",
	Usage:       "Run a script in a space",
	Description: "Execute a script in a specific space. Usage: script run <space-name> <script-name> [key=value ...]",
	MaxArgs:     cli.UnlimitedArgs,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space-name",
			Usage:    "Name of the space",
			Required: true,
		},
		&cli.StringArg{
			Name:     "script-name",
			Usage:    "Name of the script to run",
			Required: true,
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		space, err := client.GetSpaceByName(ctx, cmd.GetStringArg("space-name"))
		if err != nil {
			return fmt.Errorf("error getting space: %w", err)
		}

		script, err := client.GetScriptByName(ctx, cmd.GetStringArg("script-name"))
		if err != nil {
			return fmt.Errorf("error getting script: %w", err)
		}

		result, err := client.ExecuteScript(ctx, space.SpaceId, script.Id, cmd.GetArgs())
		if err != nil {
			return fmt.Errorf("error executing script: %w", err)
		}

		if result != "" {
			fmt.Println(result)
		}
		return nil
	},
}
