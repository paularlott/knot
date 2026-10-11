package command_stack

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var ValidateCmd = &cli.Command{
	Name:        "validate",
	Usage:       "Validate a stack definition file",
	Description: "Validate a stack definition TOML or JSON file without creating it.",
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

		result, _, err := client.ValidateStackDefinition(ctx, req)
		if err != nil {
			return fmt.Errorf("couldn't validate the stack definition: %w", err)
		}

		if result.Valid {
			fmt.Println("Stack definition is valid.")
			return nil
		}

		fmt.Printf("Stack definition has %d error(s):\n", len(result.Errors))
		for _, e := range result.Errors {
			if e.Space != "" {
				fmt.Printf("  [%s] %s: %s\n", e.Space, e.Field, e.Message)
			} else {
				fmt.Printf("  %s: %s\n", e.Field, e.Message)
			}
		}
		os.Exit(1)
		return nil
	},
}
