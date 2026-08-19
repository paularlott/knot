package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"

	"github.com/paularlott/cli"
)

var JobsDisableCmd = &cli.Command{
	Name:        "disable",
	Usage:       "Disable the job runner of a space",
	Description: "Stop a space's scheduled jobs from firing. Manual triggering still works.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")

		// Get the space ID from the space name
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		spaces, _, err := client.GetSpaces(ctx, "", false)
		if err != nil {
			return fmt.Errorf("failed to get spaces: %w", err)
		}

		var spaceId string
		for _, s := range spaces.Spaces {
			if s.Name == spaceName {
				spaceId = s.Id
				break
			}
		}

		if spaceId == "" {
			return fmt.Errorf("space '%s' not found", spaceName)
		}

		// Send the disable request
		request := &apiclient.JobSetEnabledRequest{
			Enabled: false,
		}

		response, code, err := client.SetJobEnabled(ctx, spaceId, request)
		if err != nil {
			if code == 401 {
				return fmt.Errorf("failed to authenticate with server, check token")
			} else if code == 403 {
				return fmt.Errorf("no permission to update jobs")
			} else if code == 404 {
				return fmt.Errorf("space not found")
			} else if code == 409 {
				return fmt.Errorf("space is not running")
			}
			return fmt.Errorf("failed to disable job runner: %w", err)
		}
		if !response.Success {
			return fmt.Errorf("failed to disable job runner: %s", response.Error)
		}

		fmt.Printf("Job runner disabled in space '%s'.\n", spaceName)
		return nil
	},
}
