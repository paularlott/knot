package command_templates

import (
	"context"
	"fmt"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/cli"
)

var ListCmd = &cli.Command{
	Name:        "list",
	Usage:       "List the available templates",
	Description: "Lists the available templates within the system.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		templates, _, err := client.GetTemplates(context.Background())
		if err != nil {
			return fmt.Errorf("Error getting templates: %w", err)
		}

		data := [][]string{{"Name", "Description"}}
		for _, template := range templates.Templates {
			desc := strings.ReplaceAll(template.Description, "\n", " ")
			data = append(data, []string{template.Name, desc})
		}

		util.PrintTable(data)
		return nil
	},
}
