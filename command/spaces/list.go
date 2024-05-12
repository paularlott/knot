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
			ports := make([]string, 0)

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

				// The list of HTTP ports
				for _, port := range state.HttpPorts {
					ports = append(ports, fmt.Sprintf("%d", port))
				}

				// The list of TCP ports
				for _, port := range state.TcpPorts {
					ports = append(ports, fmt.Sprintf("%d", port))
				}
			}

			// Join the ports array into a comma separated string
			portText := ""
			if len(ports) > 0 {
				portText = ports[0]
				for i := 1; i < len(ports); i++ {
					portText = portText + ", " + ports[i]
				}
			}

			data = append(data, []string{space.Name, space.TemplateName, space.Location, status, portText})
		}

		util.PrintTable(data)
	},
}
