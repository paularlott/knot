package command_stack

import (
	"context"
	"fmt"

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
			return err
		}

		req, err := loadStackDef(ctx, cmd.GetStringArg("file"), client)
		if err != nil {
			return fmt.Errorf("couldn't read the stack definition: %w", err)
		}

		// Fail if a definition with this name already exists
		existing, err := client.GetStackDefinitionByName(ctx, req.Name)
		if err != nil {
			return fmt.Errorf("couldn't check whether stack definition %q exists: %w", req.Name, err)
		}
		if existing != nil {
			return fmt.Errorf("stack definition %q already exists; update it with `knot stack apply`", req.Name)
		}

		_, _, err = client.CreateStackDefinition(ctx, req)
		if err != nil {
			return fmt.Errorf("couldn't create stack definition %q: %w", req.Name, err)
		}

		fmt.Printf("Stack definition %q created.\n", req.Name)
		return nil
	},
}
