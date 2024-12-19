package command_spaces

import (
	"fmt"

	"github.com/paularlott/knot/apiclient"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var startCmd = &cobra.Command{
	Use:   "start <space> [flags]",
	Short: "Start a space",
	Long:  `Start the named space.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting space: ", args[0])

		client := apiclient.NewClient(viper.GetString("client.server"), viper.GetString("client.token"), viper.GetBool("tls_skip_verify"))

		// Get the current user
		user, err := client.WhoAmI()
		if err != nil {
			fmt.Println("Error getting user: ", err)
			return
		}

		// Get a list of available spaces
		spaces, _, err := client.GetSpaces(user.Id)
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

		// Start the space
		code, err := client.StartSpace(spaceId)
		if err != nil {
			if code == 503 {
				fmt.Println("Cannot start space as outside of schedule")
			} else if code == 507 {
				fmt.Println("Cannot start space as resource quota exceeded")
			} else {
				fmt.Println("Error starting space: ", err)
			}

			return
		}

		fmt.Println("Space started: ", args[0])
	},
}
