package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"

	"github.com/paularlott/cli"
)

var JobsDisableCmd = &cli.Command{
	Name:        "disable",
	Usage:       "Disable the job runner of a space",
	Description: "Stop a space's scheduled jobs from firing. Manual triggering still works. Persisted on the space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		return setJobRunner(ctx, cmd, false)
	},
}

// setJobRunner flips the space's persisted runner state, keeping the job
// definitions unchanged.
func setJobRunner(ctx context.Context, cmd *cli.Command, enabled bool) error {
	spaceName := cmd.GetStringArg("space")

	client, err := jobsClient(cmd)
	if err != nil {
		return err
	}

	spaceId, err := jobsSpaceId(ctx, client, spaceName)
	if err != nil {
		return err
	}

	definitions, code, err := client.GetSpaceJobs(ctx, spaceId)
	if err != nil {
		if code == 404 {
			return fmt.Errorf("space not found")
		}
		return fmt.Errorf("failed to get jobs: %w", err)
	}

	_, code, err = client.UpdateSpaceJobs(ctx, spaceId, &apiclient.SpaceJobsRequest{
		Jobs:    definitions.Jobs,
		Enabled: enabled,
	})
	if err != nil {
		if code == 403 {
			return fmt.Errorf("no permission to update jobs")
		} else if code == 404 {
			return fmt.Errorf("space not found")
		}
		return fmt.Errorf("failed to update job runner: %w", err)
	}

	fmt.Printf("Job runner %s in space '%s'.\n", map[bool]string{true: "enabled", false: "disabled"}[enabled], spaceName)
	return nil
}
