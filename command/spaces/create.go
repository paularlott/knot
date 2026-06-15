package command_spaces

import (
	"context"
	"fmt"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

func parseCustomFields(rawFields []string) ([]apiclient.CustomFieldValue, error) {
	customFields := make([]apiclient.CustomFieldValue, 0, len(rawFields))

	for _, rawField := range rawFields {
		name, value, ok := strings.Cut(rawField, "=")
		if !ok {
			return nil, fmt.Errorf("invalid custom field %q, expected name=value", rawField)
		}

		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("invalid custom field %q, name is required", rawField)
		}

		customFields = append(customFields, apiclient.CustomFieldValue{
			Name:  name,
			Value: value,
		})
	}

	return customFields, nil
}

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
		&cli.StringSliceFlag{
			Name:  "custom-field",
			Usage: "Custom field as name=value (can be specified multiple times).",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {

		// Check shell is one of bash,zsh,fish,sh
		shell := cmd.GetString("shell")
		if shell != "bash" && shell != "zsh" && shell != "fish" && shell != "sh" {
			return fmt.Errorf("Invalid shell: %s", shell)
		}

		customFields, err := parseCustomFields(cmd.GetStringSlice("custom-field"))
		if err != nil {
			return err
		}

		fmt.Println("Creating space: ", cmd.GetStringArg("space"), " from template: ", cmd.GetStringArg("template"))

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
			if template.Name == cmd.GetStringArg("template") {
				templateId = template.Id
				break
			}
		}

		if templateId == "" {
			return fmt.Errorf("Template not found: %s", cmd.GetStringArg("template"))
		}

		// Create the template
		space := &apiclient.SpaceRequest{
			Name:         cmd.GetStringArg("space"),
			Description:  "",
			TemplateId:   templateId,
			Shell:        shell,
			UserId:       "",
			AltNames:     []model.AltNameEntry{},
			CustomFields: customFields,
		}

		_, _, err = client.CreateSpace(context.Background(), space)
		if err != nil {
			return fmt.Errorf("Error creating space: %w", err)
		}

		fmt.Println("Space created: ", cmd.GetStringArg("space"))
		return nil
	},
}
