package space

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var SpaceNoteCmd = &cli.Command{
	Name:        "set-note",
	Usage:       "Set a Note",
	Description: "Set the runtime note of the space. Allows a note to be written for the space which is shown on the dashboard along with the user entered description.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "note",
			Usage:    "The note text to set",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		noteRequest := agentlink.SpaceNoteRequest{
			Note: cmd.GetStringArg("note"),
		}

		err := agentlink.SendWithResponseMsg(agentlink.CommandSpaceNote, &noteRequest, nil)
		if err != nil {
			return fmt.Errorf("error setting space note: %w", err)
		}

		fmt.Println("Space note set.")
		return nil
	},
}
