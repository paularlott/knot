package command_stack

import (
	"context"
	"fmt"
	"os"

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

		_, err = client.DeleteStackDefinition(ctx, def.Id)
		if err != nil {
			fmt.Println("Error deleting stack definition:", err)
			os.Exit(1)
		}

		fmt.Printf("Stack definition %q deleted.\n", name)
		return nil
	},
}
