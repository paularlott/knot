package space

import (
	"fmt"
	"os"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/spf13/cobra"
)

var SpaceNoteCmd = &cobra.Command{
	Use:   `set-note <note>`,
	Short: "Set the runtime note of the space",
	Long:  `Allows a note to be written for the space which is shown on the dashboard along with the user entered description.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		noteRequest := agentlink.SpaceNoteRequest{
			Note: args[0],
		}

		err := agentlink.SendWithResponseMsg(agentlink.CommandSpaceNote, &noteRequest, nil)
		if err != nil {
			fmt.Println("Error setting space note: ", err)
			os.Exit(1)
		}

		fmt.Println("Space note set.")
	},
}
