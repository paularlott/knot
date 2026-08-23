package command_spaces

import (
	"context"

	"github.com/paularlott/cli"
)

var JobsEnableCmd = &cli.Command{
	Name:        "enable",
	Usage:       "Enable the job runner of a space",
	Description: "Start a space's scheduled jobs firing. Persisted on the space and pushed to the agent.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		return setJobRunner(ctx, cmd, true)
	},
}
