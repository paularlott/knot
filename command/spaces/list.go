package command_spaces

import (
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/util"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List the available spaces and their status",
	Long:  `Lists the available spaces for the logged in user and the state of each space.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {

		client := apiclient.NewClient(viper.GetString("client.server"), viper.GetString("client.token"), viper.GetBool("tls_skip_verify"))

		// Get the current user
		user, err := client.WhoAmI()
		if err != nil {
			fmt.Println("Error getting user: ", err)
			return
		}

		spaces, _, err := client.GetSpaces(user.Id)
		if err != nil {
			fmt.Println("Error getting spaces: ", err)
			return
		}

		data := [][]string{}
		data = append(data, []string{"Name", "Template", "Location", "Status", "Ports"})
		for _, space := range spaces.Spaces {
			status := ""
			ports := ""

			// Get the status for the space
			state, code, err := client.GetSpaceServiceState(space.Id)
			if err != nil {

				fmt.Println(code, space.Id)

				fmt.Println("Error getting space state: ", err)
				return
			}

			if state != nil {
				if state.IsRemote {
					status = "Remote"
				} else if state.IsDeployed {
					if state.IsPending {
						status = "Stopping"
					} else {
						status = "Running"
					}
				} else if state.IsDeleting {
					status = "Deleting"
				} else if state.IsPending {
					status = "Starting"
				}

				// The list of TCP ports
				for i, port := range state.TcpPorts {
					if i > 0 {
						ports += ", "
					}
					ports += fmt.Sprintf("%d", port)
				}
			}

			data = append(data, []string{space.Name, space.TemplateName, space.Location, status, ports})
		}

		util.PrintTable(data)
	},
}
