package command_spaces

import (
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var stopCmd = &cobra.Command{
	Use:   "stop <space> [flags]",
	Short: "Stop a space",
	Long:  `Stop the named space.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Stopping space: ", args[0])

		alias, _ := cmd.Flags().GetString("alias")
		cfg := config.GetServerAddr(alias)
		client := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, viper.GetBool("tls_skip_verify"))

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

		// Stop the space
		_, err = client.StopSpace(spaceId)
		if err != nil {
			fmt.Println("Error stopping space: ", err)
			return
		}

		fmt.Println("Space stopped: ", args[0])
	},
}
