package command_stack

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var EnableCmd = &cli.Command{
	Name:        "enable-def",
	Usage:       "Enable a stack definition",
	Description: "Enable a stack definition so it can be used to create stacks.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "Name of the stack definition to enable",
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

		if def.Active {
			fmt.Printf("Stack definition %q is already enabled.\n", name)
			return nil
		}

		_, err = client.UpdateStackDefinition(ctx, def.Id, &apiclient.StackDefinitionRequest{
			Name:        def.Name,
			Description: def.Description,
			Active:      true,
			Scope:       def.Scope,
			Groups:      def.Groups,
			Zones:       def.Zones,
			Spaces:      def.Spaces,
		})
		if err != nil {
			return fmt.Errorf("couldn't enable stack definition %q: %w", name, err)
		}

		fmt.Printf("Stack definition %q enabled.\n", name)
		return nil
	},
}
