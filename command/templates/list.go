package command_templates

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/util"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List the available templates",
	Long:  `Lists the available templates within the system.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {

		client := apiclient.NewClient(viper.GetString("client.server"), viper.GetString("client.token"), viper.GetBool("tls_skip_verify"))

		templates, _, err := client.GetTemplates()
		if err != nil {
			fmt.Println("Error getting templates: ", err)
			return
		}

		data := [][]string{}

		data = append(data, []string{"Name", "Description"})
		for _, template := range *templates {
			desc := strings.ReplaceAll(template.Description, "\n", " ")
			data = append(data, []string{template.Name, desc})
		}

		util.PrintTable(data)
	},
}
