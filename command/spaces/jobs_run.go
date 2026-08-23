package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"

	"github.com/paularlott/cli"
)

var JobsRunCmd = &cli.Command{
	Name:        "run",
	Usage:       "Run a job of a space now",
	Description: "Trigger a job immediately by name. Works for disabled and manual-only jobs too.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space",
			Required: true,
		},
		&cli.StringArg{
			Name:     "job",
			Usage:    "The name of the job to run",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		jobName := cmd.GetStringArg("job")

		client, err := jobsClient(cmd)
		if err != nil {
			return err
		}

		spaceId, err := jobsSpaceId(ctx, client, spaceName)
		if err != nil {
			return err
		}

		// Send the job run request
		request := &apiclient.JobRunRequest{
			Name: jobName,
		}

		response, code, err := client.RunJob(ctx, spaceId, request)
		if err != nil {
			if code == 401 {
				return fmt.Errorf("failed to authenticate with server, check token")
			} else if code == 403 {
				return fmt.Errorf("no permission to run commands")
			} else if code == 404 {
				return fmt.Errorf("space not found")
			} else if code == 409 {
				return fmt.Errorf("space is not running")
			}
			return fmt.Errorf("failed to run job: %w", err)
		}
		if !response.Success {
			return fmt.Errorf("failed to run job: %s", response.Error)
		}

		fmt.Printf("Job '%s' started in space '%s'.\n", jobName, spaceName)
		return nil
	},
}
