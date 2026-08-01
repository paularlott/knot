package command_templates

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/cli"
)

var ExportCmd = &cli.Command{
	Name:        "export",
	Usage:       "Export a template as portable YAML",
	Description: "Exports a template to a portable YAML format suitable for version control and cross-instance import. Pipe to a file with: knot template export name > template.yaml",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "The template name (or ID) to export.",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		name := cmd.GetStringArg("name")
		yaml, code, err := client.ExportTemplate(ctx, name)
		if err != nil {
			if code == 404 {
				return fmt.Errorf("template not found: %s", name)
			}
			return fmt.Errorf("failed to export template: %w", err)
		}

		fmt.Print(yaml)
		return nil
	},
}
