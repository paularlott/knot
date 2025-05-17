package command_templates

import (
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <name> [flags]",
	Short: "Delete a template",
	Long:  `Delete a template.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {

		// Prompt the user to confirm the deletion
		var confirm string
		fmt.Printf("Are you sure you want to delete the template %s? (yes/no): ", args[0])
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Deletion cancelled.")
			return
		}

		alias, _ := cmd.Flags().GetString("alias")
		cfg := config.GetServerAddr(alias)
		client := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, viper.GetBool("tls_skip_verify"))

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

		// Delete the template
		_, err = client.DeleteTemplate(templateId)
		if err != nil {
			fmt.Println("Error deleting template: ", err)
			return
		}

		fmt.Println("Template deleted: ", args[0])
	},
}
