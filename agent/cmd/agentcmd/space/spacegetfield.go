package space

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var SpaceGetFieldCmd = &cli.Command{
	Name:        "get-field",
	Usage:       "Get a Custom Field",
	Description: "Get a custom field value from the space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "The name of the custom field to get",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		varRequest := agentlink.SpaceGetFieldRequest{
			Name: cmd.GetStringArg("name"),
		}

		var response agentlink.SpaceGetFieldResponse
		err := agentlink.SendWithResponseMsg(agentlink.CommandSpaceGetField, &varRequest, &response)
		if err != nil {
			return fmt.Errorf("error getting custom field: %w", err)
		}

		fmt.Println(response.Value)
		return nil
	},
}
