package command_spaces

import (
	"fmt"

	"github.com/paularlott/knot/apiclient"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <space> [flags]",
	Short: "Delete a space",
	Long:  `Delete a stopped space, all data will be lost.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		// Prompt the user to confirm the deletion
		var confirm string
		fmt.Printf("Are you sure you want to delete the space %s and all data? (yes/no): ", args[0])
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Deletion cancelled.")
			return
		}

		client := apiclient.NewClient(viper.GetString("client.server"), viper.GetString("client.token"), viper.GetBool("tls_skip_verify"))

		// Get a list of available spaces
		spaces, _, err := client.GetSpaces("")
		if err != nil {
			fmt.Println("Error getting spaces: ", err)
			return
		}

		// Find the space by name
		var spaceId string = ""
		for _, space := range spaces.Spaces {
			if space.Name == args[0] {
				spaceId = space.Id
				break
			}
		}

		if spaceId == "" {
			fmt.Println("Space not found: ", args[0])
			return
		}

		// Delete the space
		_, err = client.DeleteSpace(spaceId)
		if err != nil {
			fmt.Println("Error deleting space: ", err)
			return
		}

		fmt.Println("Space deleting: ", args[0])
	},
}
