package command_spaces

import (
	"context"
	"fmt"
	"time"

	"github.com/paularlott/knot/command/cmdutil"

	"github.com/paularlott/cli"
)

var JobsListCmd = &cli.Command{
	Name:        "list",
	Usage:       "List the scheduled jobs of a space",
	Description: "List the jobs defined in a space's ~/.knot-jobs.toml with their schedule, next run and last run.",
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

		// Get the list of jobs
		response, code, err := client.ListJobs(ctx, spaceId)
		if err != nil {
			if code == 401 {
				return fmt.Errorf("failed to authenticate with server, check token")
			} else if code == 403 {
				return fmt.Errorf("no permission to list jobs")
			} else if code == 404 {
				return fmt.Errorf("space not found")
			} else if code == 409 {
				return fmt.Errorf("space is not running")
			}
			return fmt.Errorf("failed to list jobs: %w", err)
		}

		if !response.Found {
			fmt.Printf("No jobs file found in space '%s', create ~/.knot-jobs.toml in the space to define jobs.\n", spaceName)
			return nil
		}
		if response.Enabled {
			fmt.Printf("Job runner: enabled\n")
		} else {
			fmt.Printf("Job runner: disabled (scheduled jobs are not firing, run 'knot space jobs enable %s' to start)\n", spaceName)
		}
		if response.Error != "" {
			fmt.Printf("Warning: %s (using last good configuration)\n", response.Error)
		}
		if len(response.Jobs) == 0 {
			fmt.Printf("No jobs defined in space '%s'.\n", spaceName)
			return nil
		}

		fmt.Printf("Jobs in space '%s':\n", spaceName)
		for _, job := range response.Jobs {
			fmt.Printf("  %s", job.Name)
			if job.ManualOnly {
				fmt.Print(" (manual only)")
			} else if job.Schedule != "" {
				fmt.Printf(" (%s)", job.Schedule)
			}
			if !job.Enabled {
				fmt.Print(" [disabled]")
			}
			if job.Running {
				fmt.Print(" [running]")
			}
			fmt.Println()

			fmt.Printf("    command: %s\n", job.Command)
			if job.NextRun != nil {
				fmt.Printf("    next run: %s\n", job.NextRun.Local().Format(time.RFC3339))
			}
			if job.LastRun != nil {
				lastRun := fmt.Sprintf("    last run: %s (%s", job.LastRun.StartedAt.Local().Format(time.RFC3339), job.LastRun.Status)
				if job.LastRun.DurationMs > 0 {
					lastRun += fmt.Sprintf(", %s", (time.Duration(job.LastRun.DurationMs) * time.Millisecond).Round(time.Second))
				}
				fmt.Println(lastRun + ")")
			}
			if job.Error != "" {
				fmt.Printf("    error: %s\n", job.Error)
			}
		}
		return nil
	},
}
