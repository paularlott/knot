package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	"github.com/paularlott/cli"
)

var JobsUpdateCmd = &cli.Command{
	Name:        "update",
	Usage:       "Update a job of a space",
	Description: "Change a job's command, schedule or enabled state. Only the given flags are changed.",
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
			Name:  "command",
			Usage: "The shell command the job runs",
		},
		&cli.StringFlag{
			Name:  "schedule",
			Usage: `Cron expression, e.g. "*/5 * * * *"; empty string for a manual-only job`,
		},
		&cli.BoolFlag{
			Name:  "enable",
			Usage: "Enable the job so it fires automatically",
		},
		&cli.BoolFlag{
			Name:  "disable",
			Usage: "Disable the job so it does not fire automatically",
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

		err = jobsSaveDefinitions(ctx, client, spaceId, func(jobs *[]model.SpaceJob) error {
			for i := range *jobs {
				if (*jobs)[i].Name != jobName {
					continue
				}
				if cmd.HasFlag("command") {
					(*jobs)[i].Command = cmd.GetString("command")
				}
				if cmd.HasFlag("schedule") {
					(*jobs)[i].Schedule = cmd.GetString("schedule")
				}
				if cmd.GetBool("disable") {
					(*jobs)[i].Enabled = false
				} else if cmd.GetBool("enable") {
					(*jobs)[i].Enabled = true
				}
				return nil
			}
			return fmt.Errorf("job '%s' not found in space '%s'", jobName, spaceName)
		})
		if err != nil {
			return err
		}

		fmt.Printf("Job '%s' updated in space '%s'.\n", jobName, spaceName)
		return nil
	},
}

var JobsRemoveCmd = &cli.Command{
	Name:        "remove",
	Usage:       "Remove a job from a space",
	Description: "Remove a job definition from a space. Any running instance is unaffected.",
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

		err = jobsSaveDefinitions(ctx, client, spaceId, func(jobs *[]model.SpaceJob) error {
			kept := (*jobs)[:0]
			found := false
			for _, j := range *jobs {
				if j.Name == jobName {
					found = true
					continue
				}
				kept = append(kept, j)
			}
			if !found {
				return fmt.Errorf("job '%s' not found in space '%s'", jobName, spaceName)
			}
			*jobs = kept
			return nil
		})
		if err != nil {
			return err
		}

		fmt.Printf("Job '%s' removed from space '%s'.\n", jobName, spaceName)
		return nil
	},
}
