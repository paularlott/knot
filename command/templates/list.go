package command_templates

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List the available templates",
	Long:  `Lists the available templates within the system.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		alias, _ := cmd.Flags().GetString("alias")
		cfg := config.GetServerAddr(alias)
		client := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, viper.GetBool("tls_skip_verify"))

		templates, _, err := client.GetTemplates()
		if err != nil {
			fmt.Println("Error getting templates: ", err)
			return
		}

		data := [][]string{}

		data = append(data, []string{"Name", "Description"})
		for _, template := range templates.Templates {
			desc := strings.ReplaceAll(template.Description, "\n", " ")
			data = append(data, []string{template.Name, desc})
		}

		util.PrintTable(data)
	},
}
