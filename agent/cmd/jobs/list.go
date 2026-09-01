package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/spacejobs"

	"github.com/paularlott/cli"
)

var ListJobsCmd = &cli.Command{
	Name:        "list",
	Usage:       "List scheduled jobs",
	Description: "List this space's jobs with their schedule, next run and last run. Jobs are managed from the web UI, the knot CLI or the scriptling jobs library.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		var snapshot spacejobs.JobsSnapshot
		err := agentlink.SendWithResponseMsg(agentlink.CommandJobsList, nil, &snapshot)
		if err != nil {
			return fmt.Errorf("error listing jobs: %w", err)
		}

		if len(snapshot.Jobs) == 0 {
			fmt.Println("No jobs defined.")
			return nil
		}
		if snapshot.Enabled {
			fmt.Println("Job runner: enabled")
		} else {
			fmt.Println("Job runner: disabled (scheduled jobs are not firing, manual runs still work)")
		}

		for _, job := range snapshot.Jobs {
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
				fmt.Printf("    next run: %s\n", job.NextRun.Format(time.RFC3339))
			}
			if job.LastRun != nil {
				lastRun := fmt.Sprintf("    last run: %s (%s", job.LastRun.StartedAt.Format(time.RFC3339), job.LastRun.Status)
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
