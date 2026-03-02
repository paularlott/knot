package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var SetFieldCmd = &cli.Command{
	Name:        "set-field",
	Usage:       "Set a custom field on a space",
	Description: "Set or update a custom field value on an existing space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space to update",
			Required: true,
		},
		&cli.StringArg{
			Name:     "field",
			Usage:    "The name of the custom field to set",
			Required: true,
		},
		&cli.StringArg{
			Name:     "value",
			Usage:    "The value to set for the custom field",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		fieldName := cmd.GetStringArg("field")
		fieldValue := cmd.GetStringArg("value")

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		// Set the custom field (API supports both name and ID)
		_, err = client.SetSpaceCustomField(context.Background(), spaceName, fieldName, fieldValue)
		if err != nil {
			return fmt.Errorf("Error setting custom field: %w", err)
		}

		fmt.Printf("Custom field '%s' set to '%s' on space '%s'\n", fieldName, fieldValue, spaceName)
		return nil
	},
}
