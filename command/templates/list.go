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

		// Get the server zone
		pingResponse, err := client.Ping(context.Background())
		if err != nil {
			return fmt.Errorf("Error getting server info: %w", err)
		}
		zone := pingResponse.Zone

		templates, _, err := client.GetTemplates(context.Background())
		if err != nil {
			return fmt.Errorf("Error getting templates: %w", err)
		}

		data := [][]string{{"Name", "Description"}}
		for _, template := range templates.Templates {
			// Filter by zone - same logic as web UI (skip if zone is blank)
			if zone != "" && !isTemplateValidForZone(template.Zones, zone) {
				continue
			}

			desc := strings.ReplaceAll(template.Description, "\n", " ")
			data = append(data, []string{template.Name, desc})
		}

		util.PrintTable(data)
		return nil
	},
}

// isTemplateValidForZone checks if a template is valid for a given zone
// using the same logic as the web UI
func isTemplateValidForZone(zones []string, zone string) bool {
	// If no zones specified, template is valid for all zones
	if len(zones) == 0 {
		return true
	}

	// Check for negated zones first (zones starting with !)
	for _, z := range zones {
		if strings.HasPrefix(z, "!") && z[1:] == zone {
			return false
		}
	}

	// Check for positive zones - only non-negated zones
	hasPositiveZones := false
	for _, z := range zones {
		if !strings.HasPrefix(z, "!") {
			hasPositiveZones = true
			if z == zone {
				return true
			}
		}
	}

	// If there are positive zones but none matched, return false
	// If there are only negative zones (none matched), return true
	return !hasPositiveZones
}
