package scripts

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var deleteCmd = &cli.Command{
	Name:        "delete",
	Usage:       "Delete a script",
	Description: "Delete a script by name.",
	MinArgs:     1,
	MaxArgs:     1,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		args := cmd.GetArgs()
		script, err := client.GetScriptDetailsByName(ctx, args[0])
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
