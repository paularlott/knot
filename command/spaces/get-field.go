package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var GetFieldCmd = &cli.Command{
	Name:        "get-field",
	Usage:       "Get a custom field from a space",
	Description: "Get a custom field value from an existing space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space to get the field from",
			Required: true,
		},
		&cli.StringArg{
			Name:     "field",
			Usage:    "The name of the custom field to get",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		fieldName := cmd.GetStringArg("field")

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		// Get the custom field (API supports both name and ID)
		response, _, err := client.GetSpaceCustomField(context.Background(), spaceName, fieldName)
		if err != nil {
			return fmt.Errorf("Error getting custom field: %w", err)
		}

		fmt.Println(response.Value)
		return nil
	},
}
