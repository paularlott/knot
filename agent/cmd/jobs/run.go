package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var RunJobCmd = &cli.Command{
	Name:        "run",
	Usage:       "Run a job now",
	Description: "Trigger a job immediately by name. Works for disabled and manual-only jobs too.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "job",
			Usage:    "The name of the job to run",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		request := agentlink.JobsRunRequest{
			Name: cmd.GetStringArg("job"),
		}

		var response agentlink.JobsResponse
		err := agentlink.SendWithResponseMsg(agentlink.CommandJobsRun, &request, &response)
		if err != nil {
			return fmt.Errorf("error running job: %w", err)
		}
		if !response.Success {
			return errors.New(response.Error)
		}

		fmt.Printf("Job '%s' started.\n", request.Name)
		return nil
	},
}
