package command_scripts

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
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "Name of the script",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		scriptName := cmd.GetStringArg("name")
		script, err := resolveScript(ctx, cmd, client, scriptName)
		if err != nil {
			return err
		}

		err = client.DeleteScript(ctx, script.Id)
		if err != nil {
			return fmt.Errorf("error deleting script: %w", err)
		}

		fmt.Printf("Script %s deleted\n", scriptName)
		return nil
	},
}
