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

		spaces, _, err := client.GetSpaces("")
		if err != nil {
			fmt.Println("Error getting spaces: ", err)
			return
		}

		data := [][]string{}
		templateCache := make(map[string]*apiclient.TemplateDetails)

		data = append(data, []string{"Name", "Template", "Location", "Status", "Ports"})

		for _, space := range spaces {
			status := ""
			template := ""
			ports := ""

			// If space.TemplateId is not in the templateCache, get the template details
			if _, ok := templateCache[space.TemplateId]; !ok {
				templateDetails, _, err := client.GetTemplate(space.TemplateId)
				if err != nil {
					fmt.Println("Error getting template details: ", err)
					return
				}
				templateCache[space.TemplateId] = templateDetails
				template = templateDetails.Name
			} else {
				template = templateCache[space.TemplateId].Name
			}

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
					status = "Running"
				}

				// The list of TCP ports
				for i, port := range state.TcpPorts {
					if i > 0 {
						ports += ", "
					}
					ports += fmt.Sprintf("%d", port)
				}
			}

			data = append(data, []string{space.Name, template, space.Location, status, ports})
		}

		util.PrintTable(data)
	},
}
