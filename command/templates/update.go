package command_templates

import (
	"fmt"

	"github.com/paularlott/knot/apiclient"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	updateCmd.Flags().StringP("job", "j", "", "The file to load for the nomad job description.")
	updateCmd.Flags().StringP("volume", "v", "", "The YAML file to load for the volume description.")
	updateCmd.Flags().Bool("with-terminal", false, "Enable terminal for the template.")
	updateCmd.Flags().Bool("with-vscode-tunnel", false, "Enable VSCode tunnel for the template.")
	updateCmd.Flags().Bool("with-code-server", false, "Enable Code Server for the template.")
	updateCmd.Flags().Bool("with-ssh", false, "Enable SSH for the template.")
}

var updateCmd = &cobra.Command{
	Use:   "update <name> [flags]",
	Short: "Update a template",
	Long:  `Update the template job and storage definition from files.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		jobFile := cmd.Flags().Lookup("job").Value.String()
		volumeFile := cmd.Flags().Lookup("volume").Value.String()

		client := apiclient.NewClient(viper.GetString("client.server"), viper.GetString("client.token"), viper.GetBool("tls_skip_verify"))

		// Get a list of available templates
		templates, _, err := client.GetTemplates()
		if err != nil {
			fmt.Println("Error getting templates: ", err)
			return
		}

		// Find the ID of the template from the name
		var templateId string = ""
		for _, template := range templates.Templates {
			if template.Name == args[0] {
				templateId = template.Id
				break
			}
		}

		if templateId == "" {
			fmt.Println("Template not found: ", args[0])
			return
		}

		// Fetch the template
		template, _, err := client.GetTemplate(templateId)
		if err != nil {
			fmt.Println("Error getting template: ", err)
			return
		}

		// If job file given, load from file
		if jobFile != "" {
			job, err := loadFile(jobFile)
			if err != nil {
				fmt.Println("Error loading job file: ", err)
				return
			}
			template.Job = job
		}

		// If volume file given, load from file
		if volumeFile != "" {
			volume, err := loadFile(volumeFile)
			if err != nil {
				fmt.Println("Error loading volume file: ", err)
				return
			}
			template.Volumes = volume
		}

		if jobFile != "" || volumeFile != "" {
			_, err = client.UpdateTemplate(
				templateId,
				template.Name,
				template.Job,
				template.Description,
				template.Volumes,
				template.Groups,
				viper.GetBool("with-terminal"),
				viper.GetBool("with-vscode-tunnel"),
				viper.GetBool("with-code-server"),
				viper.GetBool("with-ssh"),
				0,
				0,
			)
			if err != nil {
				fmt.Println("Error updating template: ", err)
				return
			}
		}

		fmt.Println("Template updated: ", args[0])
	},
}
