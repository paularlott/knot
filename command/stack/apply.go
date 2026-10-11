package command_stack

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var ApplyCmd = &cli.Command{
	Name:        "apply",
	Usage:       "Update an existing stack definition",
	Description: "Update an existing stack definition from a TOML or JSON file.",
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

		existing, err := client.GetStackDefinitionByName(ctx, req.Name)
		if err != nil {
			return fmt.Errorf("couldn't check whether stack definition %q exists: %w", req.Name, err)
		}
		if existing == nil {
			return fmt.Errorf("stack definition %q not found; create it with `knot stack create-def`", req.Name)
		}

		_, err = client.UpdateStackDefinition(ctx, existing.Id, req)
		if err != nil {
			return fmt.Errorf("couldn't update stack definition %q: %w", req.Name, err)
		}

		fmt.Printf("Stack definition %q updated.\n", req.Name)
		return nil
	},
}
