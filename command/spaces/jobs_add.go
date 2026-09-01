package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	"github.com/paularlott/cli"
)

var JobsAddCmd = &cli.Command{
	Name:        "add",
	Usage:       "Add a job to a space",
	Description: "Add a job to a space. The schedule is a 5-field cron expression (minute hour day month weekday); omit it for a manual-only job.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space",
			Required: true,
		},
		&cli.StringArg{
			Name:     "job",
			Usage:    "The name of the job",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "command",
			Usage:    "The shell command the job runs",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "schedule",
			Usage: `Cron expression, e.g. "*/5 * * * *" or "30 9 * * 1-5"; empty for a manual-only job`,
		},
		&cli.BoolFlag{
			Name:  "disabled",
			Usage: "Add the job disabled (it will not fire automatically)",
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

		job := model.SpaceJob{
			Name:     jobName,
			Command:  cmd.GetString("command"),
			Schedule: cmd.GetString("schedule"),
			Enabled:  !cmd.GetBool("disabled"),
		}

		err = jobsSaveDefinitions(ctx, client, spaceId, func(jobs *[]model.SpaceJob) error {
			for _, j := range *jobs {
				if j.Name == job.Name {
					return fmt.Errorf("job '%s' already exists in space '%s'", job.Name, spaceName)
				}
			}
			*jobs = append(*jobs, job)
			return nil
		})
		if err != nil {
			return err
		}

		fmt.Printf("Job '%s' added to space '%s'.\n", jobName, spaceName)
		return nil
	},
}
