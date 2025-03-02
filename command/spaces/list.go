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

			if space.IsRemote {
				status = "Remote "
			}

			if space.IsDeployed {
				if space.IsPending {
					status = status + "Stopping"
				} else {
					status = status + "Running"
				}
			} else if space.IsDeleting {
				status = status + "Deleting"
			} else if space.IsPending {
				status = status + "Starting"
			}

			// The list of HTTP ports
			for port, desc := range space.HttpPorts {
				var p string

				if port == desc {
					p = port
				} else {
					p = fmt.Sprintf("%s (%s)", desc, port)
				}

				ports = append(ports, p)
			}

			// The list of TCP ports
			for port, desc := range space.TcpPorts {
				var p string

				if port == desc {
					p = port
				} else {
					p = fmt.Sprintf("%s (%s)", desc, port)
				}

				ports = append(ports, p)
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
