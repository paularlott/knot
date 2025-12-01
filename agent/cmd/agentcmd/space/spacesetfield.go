package space

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var SpaceSetFieldCmd = &cli.Command{
	Name:        "set-field",
	Usage:       "Set a Custom Field",
	Description: "Set a custom field value for the space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "The name of the custom field to set",
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
		varRequest := agentlink.SpaceFieldRequest{
			Name:  cmd.GetStringArg("name"),
			Value: cmd.GetStringArg("value"),
		}

		err := agentlink.SendWithResponseMsg(agentlink.CommandSpaceSetField, &varRequest, nil)
		if err != nil {
			return fmt.Errorf("error setting custom field: %w", err)
		}

		fmt.Println("Custom field set.")
		return nil
	},
}
