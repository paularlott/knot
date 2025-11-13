package space

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var SpaceGetVarCmd = &cli.Command{
	Name:        "get-var",
	Usage:       "Get a Variable",
	Description: "Get a custom field variable value from the space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "The name of the variable to get",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		varRequest := agentlink.SpaceGetVarRequest{
			Name: cmd.GetStringArg("name"),
		}

		var response agentlink.SpaceGetVarResponse
		err := agentlink.SendWithResponseMsg(agentlink.CommandSpaceGetVar, &varRequest, &response)
		if err != nil {
			return fmt.Errorf("error getting space variable: %w", err)
		}

		fmt.Println(response.Value)
		return nil
	},
}
