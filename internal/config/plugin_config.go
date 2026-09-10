package config

import (
	"fmt"

	"github.com/paularlott/cli"
)

// PluginConfigs extracts the [plugins.<name>] tables from the server's
// configuration file: each plugin's configuration, delivered to its
// handlers as request["config"] and validated against the plugin's
// [tool.knot] config declaration at plugin load.
//
// [plugins] must hold one table per plugin; entries that are not tables
// are skipped and reported as warnings so a malformed section is visible
// without stopping the server.
func PluginConfigs(cmd *cli.Command) (map[string]map[string]any, []string) {
	configs := map[string]map[string]any{}

	if cmd == nil || cmd.ConfigFile == nil {
		return configs, nil
	}

	raw, exists := cmd.ConfigFile.GetValue("plugins")
	if !exists || raw == nil {
		return configs, nil
	}

	table, ok := raw.(map[string]any)
	if !ok {
		return configs, []string{"[plugins] in the configuration must hold one table per plugin, e.g. [plugins.my-plugin]"}
	}

	var warnings []string
	for name, value := range table {
		entry, ok := value.(map[string]any)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("[plugins.%s] is not a table, skipped", name))
			continue
		}
		configs[name] = entry
	}
	return configs, warnings
}
