package command_spaces

import (
	"context"
	"fmt"
	"time"

	"github.com/paularlott/cli"
)

var JobsListCmd = &cli.Command{
	Name:        "list",
	Usage:       "List the scheduled jobs of a space",
	Description: "List a space's jobs with their schedule, next run and last run. Definitions come from the space record; run status needs the space to be running.",
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

		client, err := jobsClient(cmd)
		if err != nil {
			return err
		}

		spaceId, err := jobsSpaceId(ctx, client, spaceName)
		if err != nil {
			return err
		}

		// Live snapshot from the agent when the space is running, for next
		// run / last run / running state; otherwise fall back to the
		// persisted definitions alone.
		live, _, liveErr := client.ListJobs(ctx, spaceId)
		if liveErr != nil {
			live = nil
		}

		definitions, code, err := client.GetSpaceJobs(ctx, spaceId)
		if err != nil {
			if code == 401 {
				return fmt.Errorf("failed to authenticate with server, check token")
			} else if code == 403 {
				return fmt.Errorf("no permission to list jobs")
			} else if code == 404 {
				return fmt.Errorf("space not found")
			}
			return fmt.Errorf("failed to list jobs: %w", err)
		}

		if len(definitions.Jobs) == 0 {
			fmt.Printf("No jobs defined in space '%s'.\n", spaceName)
			return nil
		}

		fmt.Printf("Jobs in space '%s':\n", spaceName)
		if definitions.Enabled {
			fmt.Printf("  job runner: enabled\n")
		} else {
			fmt.Printf("  job runner: disabled (scheduled jobs are not firing, manual runs still work)\n")
		}

		for _, job := range definitions.Jobs {
			fmt.Printf("  %s", job.Name)
			if job.Schedule == "" {
				fmt.Print(" (manual only)")
			} else {
				fmt.Printf(" (%s)", job.Schedule)
			}
			if !job.Enabled {
				fmt.Print(" [disabled]")
			}
			fmt.Println()

			fmt.Printf("    command: %s\n", job.Command)

			if live != nil {
				for _, status := range live.Jobs {
					if status.Name != job.Name {
						continue
					}
					if status.Running {
						fmt.Printf("    running now\n")
					}
					if status.NextRun != nil {
						fmt.Printf("    next run: %s\n", status.NextRun.Local().Format(time.RFC3339))
					}
					if status.LastRun != nil {
						lastRun := fmt.Sprintf("    last run: %s (%s", status.LastRun.StartedAt.Local().Format(time.RFC3339), status.LastRun.Status)
						if status.LastRun.DurationMs > 0 {
							lastRun += fmt.Sprintf(", %s", (time.Duration(status.LastRun.DurationMs) * time.Millisecond).Round(time.Second))
						}
						fmt.Println(lastRun + ")")
					}
					if status.Error != "" {
						fmt.Printf("    error: %s\n", status.Error)
					}
				}
			}
		}
		if live == nil && liveErr != nil {
			fmt.Printf("\n(space not running, showing definitions without run status)\n")
		}
		return nil
	},
}
