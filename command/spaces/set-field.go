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

		// Get the current user
		user, err := client.WhoAmI(context.Background())
		if err != nil {
			return fmt.Errorf("Error getting user: %w", err)
		}

		// Get a list of available spaces
		spaces, _, err := client.GetSpaces(context.Background(), user.Id)
		if err != nil {
			return fmt.Errorf("Error getting spaces: %w", err)
		}

		// Find the space by name
		var spaceId string
		for _, space := range spaces.Spaces {
			if space.Name == spaceName {
				spaceId = space.Id
				break
			}
		}

		if spaceId == "" {
			return fmt.Errorf("Space not found: %s", spaceName)
		}

		// Set the custom field using the dedicated endpoint
		_, err = client.SetSpaceCustomField(context.Background(), spaceId, fieldName, fieldValue)
		if err != nil {
			return fmt.Errorf("Error setting custom field: %w", err)
		}

		fmt.Printf("Custom field '%s' set to '%s' on space '%s'\n", fieldName, fieldValue, spaceName)
		return nil
	},
}
