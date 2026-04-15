package command_stack

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var CreateDefCmd = &cli.Command{
	Name:        "create-def",
	Usage:       "Create a new stack definition",
	Description: "Create a new stack definition from a TOML or JSON file.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "file",
			Usage:    "Path to the TOML or JSON definition file",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		req, err := loadStackDef(ctx, cmd.GetStringArg("file"), client)
		if err != nil {
			fmt.Println("Error reading definition:", err)
			os.Exit(1)
		}

		// Fail if a definition with this name already exists
		existing, err := client.GetStackDefinitionByName(ctx, req.Name)
		if err != nil {
			fmt.Println("Error checking for existing definition:", err)
			os.Exit(1)
		}
		if existing != nil {
			fmt.Printf("Stack definition %q already exists. Use 'apply' to update it.\n", req.Name)
			os.Exit(1)
		}

		_, _, err = client.CreateStackDefinition(ctx, req)
		if err != nil {
			fmt.Println("Error creating stack definition:", err)
			os.Exit(1)
		}

		fmt.Printf("Stack definition %q created.\n", req.Name)
		return nil
	},
}
