package command_stack

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var DisableCmd = &cli.Command{
	Name:        "disable-def",
	Usage:       "Disable a stack definition",
	Description: "Disable a stack definition so it cannot be used to create stacks.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "Name of the stack definition to disable",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		name := cmd.GetStringArg("name")

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		def, err := client.GetStackDefinitionByName(ctx, name)
		if err != nil {
			fmt.Println("Error looking up stack definition:", err)
			os.Exit(1)
		}
		if def == nil {
			fmt.Printf("Stack definition %q not found.\n", name)
			os.Exit(1)
		}

		if !def.Active {
			fmt.Printf("Stack definition %q is already disabled.\n", name)
			return nil
		}

		_, err = client.UpdateStackDefinition(ctx, def.Id, &apiclient.StackDefinitionRequest{
			Name:        def.Name,
			Description: def.Description,
			IconURL:     def.IconURL,
			Active:      false,
			Scope:       def.Scope,
			Groups:      def.Groups,
			Zones:       def.Zones,
			Spaces:      def.Spaces,
		})
		if err != nil {
			fmt.Println("Error disabling stack definition:", err)
			os.Exit(1)
		}

		fmt.Printf("Stack definition %q disabled.\n", name)
		return nil
	},
}
