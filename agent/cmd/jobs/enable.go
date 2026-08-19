package jobs

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
)

var EnableJobCmd = &cli.Command{
	Name:        "enable",
	Usage:       "Enable the job runner",
	Description: "Start running scheduled jobs. Requires ~/.knot-jobs.toml to exist.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		if err := setJobRunnerEnabled(true); err != nil {
			return err
		}
		fmt.Println("Job runner enabled.")
		return nil
	},
}
