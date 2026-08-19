package command_spaces

import (
	"github.com/paularlott/cli"
)

// JobsCmd is the `knot space jobs` group: view and trigger the scheduled jobs
// defined by ~/.knot-jobs.toml inside a space. These commands also work from
// inside a space: cmdutil.GetClient picks up the agent's own credentials via
// the agentlink socket.
var JobsCmd = &cli.Command{
	Name:        "jobs",
	Usage:       "Manage a space's scheduled jobs",
	Description: `Manage the jobs defined in a space's ~/.knot-jobs.toml.`,
	MaxArgs:     cli.NoArgs,
	Commands: []*cli.Command{
		JobsListCmd,
		JobsRunCmd,
		JobsEnableCmd,
		JobsDisableCmd,
	},
}
