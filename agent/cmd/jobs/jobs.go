package jobs

import (
	"github.com/paularlott/cli"
)

var JobsCmd = &cli.Command{
	Name:        "jobs",
	Usage:       "Inspect scheduled jobs",
	Description: "Inspect and trigger the jobs defined for this space. Jobs are managed from the web UI, the knot CLI or the scriptling jobs library.",
	MaxArgs:     cli.NoArgs,
	Commands: []*cli.Command{
		ListJobsCmd,
		RunJobCmd,
	},
}
