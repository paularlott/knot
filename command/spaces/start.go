package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var StartCmd = &cli.Command{
	Name:        "start",
	Usage:       "Start a space",
	Description: "Start the named space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space to start",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		fmt.Println("Starting space: ", spaceName)

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

		// Start the space
		code, err := client.StartSpace(context.Background(), spaceId)
		if err != nil {
			if code == 503 {
				return fmt.Errorf("Cannot start space as outside of schedule")
			} else if code == 507 {
				return fmt.Errorf("Cannot start space as resource quota exceeded")
			} else {
				return fmt.Errorf("Error starting space: %w", err)
			}
		}

		fmt.Println("Space started: ", spaceName)
		return nil
	},
}
