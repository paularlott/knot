package command_spaces

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	createCmd.Flags().StringP("shell", "", "bash", "The shell to use for the space (sh, bash, zsh or fish).\nOverrides the "+config.CONFIG_ENV_PREFIX+"_SHELL environment variable if set.")
}

var createCmd = &cobra.Command{
	Use:   "create <space> <template> [flags]",
	Short: "Create a space",
	Long:  `Create a new space from the given template. The new space is not started automatically.`,
	Args:  cobra.ExactArgs(2),
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("shell", cmd.Flags().Lookup("shell"))
		viper.BindEnv("shell", config.CONFIG_ENV_PREFIX+"_SHELL")
		viper.SetDefault("shell", "bash")
	},
	Run: func(cmd *cobra.Command, args []string) {

		// Check shell is one of bash,zsh,fish,sh
		shell := viper.GetString("shell")
		if shell != "bash" && shell != "zsh" && shell != "fish" && shell != "sh" {
			fmt.Println("Invalid shell: ", shell)
			return
		}

		fmt.Println("Creating space: ", args[0], " from template: ", args[1])

		alias, _ := cmd.Flags().GetString("alias")
		cfg := config.GetServerAddr(alias)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, viper.GetBool("tls_skip_verify"))
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		// Get a list of available templates
		templates, _, err := client.GetTemplates(context.Background())
		if err != nil {
			fmt.Println("Error getting templates: ", err)
			return
		}

		// Find the ID of the template from the name
		var templateId string = ""
		for _, template := range templates.Templates {
			if template.Name == args[1] {
				templateId = template.Id
				break
			}
		}

		if templateId == "" {
			fmt.Println("Template not found: ", args[1])
			return
		}

		// Create the template
		space := &apiclient.SpaceRequest{
			Name:        args[0],
			Description: "",
			TemplateId:  templateId,
			Shell:       shell,
			UserId:      "",
			AltNames:    []string{},
		}

		_, _, err = client.CreateSpace(context.Background(), space)
		if err != nil {
			fmt.Println("Error creating space: ", err)
			return
		}

		fmt.Println("Space created: ", args[0])
	},
}
