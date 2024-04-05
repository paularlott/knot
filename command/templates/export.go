package command_templates

import (
	"fmt"
	"os"

	"github.com/paularlott/knot/apiclient"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	exportCmd.Flags().StringP("job", "j", "", "The file to save the nomad job description in.")
	exportCmd.Flags().StringP("volume", "v", "", "The YAML file to save the volume description in.")
}

var exportCmd = &cobra.Command{
	Use:   "export <name> [flags]",
	Short: "Export a template",
	Long:  `Export the template job and storage definition to files.`,
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
		for _, template := range *templates {
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

		// If job file given, write to file
		if jobFile != "" {
			err = saveFile(jobFile, template.Job)
			if err != nil {
				fmt.Println("Error saving job file: ", err)
				return
			}
		}

		// If volume file given, write to file
		if volumeFile != "" {
			err = saveFile(volumeFile, template.Volumes)
			if err != nil {
				fmt.Println("Error saving volume file: ", err)
				return
			}
		}

		fmt.Println("Template exported: ", args[0])
	},
}

func saveFile(file string, data string) error {

	// Write the data to the file
	err := os.WriteFile(file, []byte(data), 0644)
	return err
}
