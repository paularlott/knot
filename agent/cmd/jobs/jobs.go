package jobs

import (
	"github.com/paularlott/cli"
)

var JobsCmd = &cli.Command{
	Name:        "jobs",
	Usage:       "Manage scheduled jobs",
	Description: "Manage the jobs defined in ~/.knot-jobs.toml.",
	MaxArgs:     cli.NoArgs,
	Commands: []*cli.Command{
		ListJobsCmd,
		RunJobCmd,
		EnableJobCmd,
		DisableJobCmd,
	},
}
