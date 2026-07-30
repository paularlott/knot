package command_templates

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/cli"
)

var ImportCmd = &cli.Command{
	Name:        "import",
	Usage:       "Import a template from portable YAML",
	Description: "Imports a template from a YAML file (or stdin). Creates a new template on the server. Scripts are resolved by name.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Path to a YAML file to import. If omitted, reads from stdin.",
		},
		&cli.StringFlag{
			Name:  "name",
			Usage: "Override the template name from the YAML file.",
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

		// Read YAML from file or stdin.
		var data []byte
		filePath := cmd.GetString("file")
		if filePath != "" {
			data, err = os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}
		} else {
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) != 0 {
				return fmt.Errorf("no input: provide --file or pipe YAML via stdin")
			}
			data, err = io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
		}

		// If --name override, patch the YAML before sending.
		nameOverride := cmd.GetString("name")
		if nameOverride != "" {
			data = patchYamlName(data, nameOverride)
		}

		id, code, err := client.ImportTemplate(ctx, string(data))
		if err != nil {
			return fmt.Errorf("failed to import template: %w", err)
		}

		if code == 200 {
			fmt.Printf("Template updated successfully (ID: %s)\n", id)
		} else {
			fmt.Printf("Template imported successfully (ID: %s)\n", id)
		}
		return nil
	},
}

// patchYamlName replaces or inserts the top-level name: field in the YAML text.
// This is a simple text replacement to avoid a full YAML round-trip.
func patchYamlName(data []byte, name string) []byte {
	lines := splitLines(string(data))
	replaced := false
	for i, line := range lines {
		if len(line) > 4 && line[:5] == "name:" {
			lines[i] = fmt.Sprintf("name: %q", name)
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append([]string{fmt.Sprintf("name: %q", name)}, lines...)
	}
	return []byte(joinLines(lines))
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}
