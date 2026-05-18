package command_stack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paularlott/cli"
	cli_toml "github.com/paularlott/cli/toml"
	"github.com/paularlott/knot/apiclient"
)

// loadStackDef loads a stack definition from a TOML or JSON file, auto-detected by extension.
func loadStackDef(ctx context.Context, filePath string, client *apiclient.ApiClient) (*apiclient.StackDefinitionRequest, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".toml":
		return loadStackDefFromTOML(ctx, filePath, client)
	case ".json":
		return loadStackDefFromJSON(filePath)
	default:
		return nil, fmt.Errorf("unsupported file format: %s (use .toml or .json)", ext)
	}
}

// loadStackDefFromJSON reads a JSON file directly into a StackDefinitionRequest.
// JSON files use IDs directly (template_id, startup_script_id, group IDs) matching the API wire format.
func loadStackDefFromJSON(filePath string) (*apiclient.StackDefinitionRequest, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	req := &apiclient.StackDefinitionRequest{
		Active: true,
	}
	if err := json.Unmarshal(data, req); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if req.Name == "" {
		return nil, fmt.Errorf("stack definition must have a name")
	}
	if req.Scope == "" {
		req.Scope = "user"
	}

	return req, nil
}

// loadStackDefFromTOML reads a TOML file and resolves template/script/group names to IDs.
func loadStackDefFromTOML(ctx context.Context, filePath string, client *apiclient.ApiClient) (*apiclient.StackDefinitionRequest, error) {
	cfg := cli.NewTypedConfigFile(cli_toml.NewConfigFile(cli.StrToPtr(filePath), nil))
	if err := cfg.LoadData(); err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	req := &apiclient.StackDefinitionRequest{
		Name:        cfg.GetString("name"),
		Description: cfg.GetString("description"),
		Active:      true,
		Scope:       cfg.GetString("scope"),
		Groups:      cfg.GetStringSlice("groups"),
		Zones:       cfg.GetStringSlice("zones"),
	}

	if req.Name == "" {
		return nil, fmt.Errorf("stack definition must have a name")
	}
	if req.Scope == "" {
		req.Scope = "user"
	}

	// Resolve group names to IDs
	if len(req.Groups) > 0 {
		groups, _, err := client.GetGroups(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch groups: %w", err)
		}
		groupMap := make(map[string]string, len(groups.Groups))
		for _, g := range groups.Groups {
			groupMap[g.Name] = g.Id
		}
		resolved := make([]string, 0, len(req.Groups))
		for _, name := range req.Groups {
			id, ok := groupMap[name]
			if !ok {
				return nil, fmt.Errorf("group not found: %s", name)
			}
			resolved = append(resolved, id)
		}
		req.Groups = resolved
	}

	// Parse spaces
	spaceDefs := cfg.GetObjectSlice("spaces")
	for _, s := range spaceDefs {
		templateName := s.GetString("template")
		if templateName == "" {
			return nil, fmt.Errorf("space %q missing template", s.GetString("name"))
		}

		tmpl, err := client.GetTemplateByName(ctx, templateName)
		if err != nil {
			return nil, fmt.Errorf("template not found: %s", templateName)
		}

		space := apiclient.StackDefSpace{
			Name:        s.GetString("name"),
			TemplateId:  tmpl.TemplateId,
			Description: s.GetString("description"),
			Shell:       s.GetString("shell"),
			DependsOn:   s.GetStringSlice("depends_on"),
		}

		if space.Name == "" {
			return nil, fmt.Errorf("each space must have a name")
		}

		// Resolve startup script name
		if scriptName := s.GetString("startup_script"); scriptName != "" {
			scripts, err := client.GetScripts(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch scripts: %w", err)
			}
			found := false
			for _, sc := range scripts.Scripts {
				if sc.Name == scriptName {
					space.StartupScript = sc.Id
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("script not found: %s", scriptName)
			}
		}

		// Custom fields
		for _, cf := range s.GetObjectSlice("custom_fields") {
			space.CustomFields = append(space.CustomFields, apiclient.StackDefCustomField{
				Name:  cf.GetString("name"),
				Value: cf.GetString("value"),
			})
		}

		// Port forwards
		for _, pf := range s.GetObjectSlice("port_forwards") {
			space.PortForwards = append(space.PortForwards, apiclient.StackDefPortForward{
				ToSpace:    pf.GetString("to_space"),
				LocalPort:  uint16(pf.GetInt("local_port")),
				RemotePort: uint16(pf.GetInt("remote_port")),
			})
		}

		req.Spaces = append(req.Spaces, space)
	}

	return req, nil
}
