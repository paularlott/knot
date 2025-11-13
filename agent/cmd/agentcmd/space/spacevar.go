package space

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var SpaceVarCmd = &cli.Command{
	Name:        "set-var",
	Usage:       "Set a Variable",
	Description: "Set a custom field variable value for the space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "The name of the variable to set",
			Required: true,
		},
		&cli.StringArg{
			Name:     "value",
			Usage:    "The value to set",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		varRequest := agentlink.SpaceVarRequest{
			Name:  cmd.GetStringArg("name"),
			Value: cmd.GetStringArg("value"),
		}

		err := agentlink.SendWithResponseMsg(agentlink.CommandSpaceVar, &varRequest, nil)
		if err != nil {
			return fmt.Errorf("error setting space variable: %w", err)
		}

		fmt.Println("Space variable set.")
		return nil
	},
}
