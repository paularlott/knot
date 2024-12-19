package command_templates

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/paularlott/knot/apiclient"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type TemplateExport struct {
	TemplateId string                     `json:"template_id"`
	Template   *apiclient.TemplateDetails `json:"template"`
}

func init() {
	exportCmd.Flags().StringP("output", "o", "", "The file to save the template in.")
}

var exportCmd = &cobra.Command{
	Use:   "export <name> [flags]",
	Short: "Export a template",
	Long:  `Export the template job and storage definition to files.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
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

		tpl := TemplateExport{
			TemplateId: templateId,
			Template:   template,
		}

		// JSON Encode the template data
		templateData, err := json.MarshalIndent(tpl, "", "  ")
		if err != nil {
			fmt.Println("Error encoding template: ", err)
			return
		}

		outputFile := cmd.Flags().Lookup("output").Value.String()
		if outputFile == "" {
			fmt.Println(string(templateData))
		} else {
			err = saveFile(outputFile, string(templateData))
			if err != nil {
				fmt.Println("Error saving template file: ", err)
				return
			}
		}
	},
}

func saveFile(file string, data string) error {

	// Write the data to the file
	err := os.WriteFile(file, []byte(data), 0644)
	return err
}
