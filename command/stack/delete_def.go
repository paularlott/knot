package command_stack

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var DeleteDefCmd = &cli.Command{
	Name:        "delete-def",
	Usage:       "Delete a stack definition",
	Description: "Delete a stack definition by name.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "Name of the stack definition to delete",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		name := cmd.GetStringArg("name")

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}

		def, err := client.GetStackDefinitionByName(ctx, name)
		if err != nil {
			return fmt.Errorf("couldn't look up stack definition %q: %w", name, err)
		}
		if def == nil {
			return fmt.Errorf("stack definition %q not found", name)
		}

		_, err = client.DeleteStackDefinition(ctx, def.Id)
		if err != nil {
			return fmt.Errorf("couldn't delete stack definition %q: %w", name, err)
		}

		fmt.Printf("Stack definition %q deleted.\n", name)
		return nil
	},
}
