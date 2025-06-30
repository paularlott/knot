package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/cli"
)

var CreateCmd = &cli.Command{
	Name:        "create",
	Usage:       "Create a space",
	Description: `Create a new space from the given template. The new space is not started automatically.`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the new space to create",
			Required: true,
		},
		&cli.StringArg{
			Name:     "template",
			Usage:    "The name of the template to use for the space",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:         "shell",
			Usage:        "The shell to use for the space (sh, bash, zsh or fish).",
			ConfigPath:   []string{"shell"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_SHELL"},
			DefaultValue: "bash",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {

		// Check shell is one of bash,zsh,fish,sh
		shell := cmd.GetString("shell")
		if shell != "bash" && shell != "zsh" && shell != "fish" && shell != "sh" {
			return fmt.Errorf("Invalid shell: %s", shell)
		}

		fmt.Println("Creating space: ", cmd.GetString("space"), " from template: ", cmd.GetString("template"))

		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		// Get a list of available templates
		templates, _, err := client.GetTemplates(context.Background())
		if err != nil {
			return fmt.Errorf("Error getting templates: %w", err)
		}

		// Find the ID of the template from the name
		var templateId string = ""
		for _, template := range templates.Templates {
			if template.Name == cmd.GetString("template") {
				templateId = template.Id
				break
			}
		}

		if templateId == "" {
			return fmt.Errorf("Template not found: %s", cmd.GetString("template"))
		}

		// Create the template
		space := &apiclient.SpaceRequest{
			Name:        cmd.GetString("space"),
			Description: "",
			TemplateId:  templateId,
			Shell:       shell,
			UserId:      "",
			AltNames:    []string{},
		}

		_, _, err = client.CreateSpace(context.Background(), space)
		if err != nil {
			return fmt.Errorf("Error creating space: %w", err)
		}

		fmt.Println("Space created: ", cmd.GetString("space"))
		return nil
	},
}
