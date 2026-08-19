package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var DisableJobCmd = &cli.Command{
	Name:        "disable",
	Usage:       "Disable the job runner",
	Description: "Stop scheduled jobs from firing. Manual triggering still works.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		if err := setJobRunnerEnabled(false); err != nil {
			return err
		}
		fmt.Println("Job runner disabled.")
		return nil
	},
}

// setJobRunnerEnabled starts or stops the job runner. The state lives in the
// agent's memory only: on the next start the runner defaults to running when
// ~/.knot-jobs.toml exists and stopped when it does not.
func setJobRunnerEnabled(enabled bool) error {
	request := agentlink.JobsSetEnabledRequest{
		Enabled: enabled,
	}

	var response agentlink.JobsResponse
	err := agentlink.SendWithResponseMsg(agentlink.CommandJobsSetEnabled, &request, &response)
	if err != nil {
		return fmt.Errorf("error updating job runner: %w", err)
	}
	if !response.Success {
		return errors.New(response.Error)
	}

	return nil
}
