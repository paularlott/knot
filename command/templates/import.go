package command_templates

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/paularlott/knot/apiclient"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var importCmd = &cobra.Command{
	Use:   "import <file> [flags]",
	Short: "Import the given template",
	Long:  `If the template already exists then it is updated otherwise it is created.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		// Load the template file and unmarshal the JSON
		data, err := os.ReadFile(args[0])
		if err != nil {
			fmt.Println("Error loading template file: ", err)
			return
		}

		template := TemplateExport{}
		err = json.Unmarshal(data, &template)
		if err != nil {
			fmt.Println("Error unmarshalling template file: ", err)
			return
		}

		client := apiclient.NewClient(viper.GetString("client.server"), viper.GetString("client.token"), viper.GetBool("tls_skip_verify"))

		// Fetch the template
		_, code, err := client.GetTemplate(template.TemplateId)
		if err != nil {
			if code == 404 {
				fmt.Println("Creating new template: ", template.Template.Name)

				_, _, err = client.CreateTemplate(
					template.Template.Name,
					template.Template.Job,
					template.Template.Description,
					template.Template.Volumes,
					template.Template.Groups,
					template.Template.LocalContainer,
					template.Template.IsManual,
					template.Template.WithTerminal,
					template.Template.WithVSCodeTunnel,
					template.Template.WithCodeServer,
					template.Template.WithSSH,
					template.Template.ComputeUnits,
					template.Template.StorageUnits,
					template.Template.ScheduleEnabled,
					&template.Template.Schedule,
					template.Template.Locations,
				)
				if err != nil {
					fmt.Println("Error creating template: ", err)
				}

			} else {
				fmt.Println("Error getting template: ", err)
			}
		} else {
			fmt.Println("Updating existing template: ", template.TemplateId)

			_, err = client.UpdateTemplate(
				template.TemplateId,
				template.Template.Name,
				template.Template.Job,
				template.Template.Description,
				template.Template.Volumes,
				template.Template.Groups,
				template.Template.WithTerminal,
				template.Template.WithVSCodeTunnel,
				template.Template.WithCodeServer,
				template.Template.WithSSH,
				template.Template.ComputeUnits,
				template.Template.StorageUnits,
				template.Template.ScheduleEnabled,
				&template.Template.Schedule,
				template.Template.Locations,
			)
			if err != nil {
				fmt.Println("Error updating template: ", err)
			}
		}
	},
}
