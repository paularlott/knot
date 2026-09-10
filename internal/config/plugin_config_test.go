package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paularlott/cli"
	cli_toml "github.com/paularlott/cli/toml"
)

func cmdWithConfigFile(t *testing.T, content string) *cli.Command {
	t.Helper()
	path := filepath.Join(t.TempDir(), "knot.toml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return &cli.Command{ConfigFile: cli_toml.NewConfigFile(&path, nil)}
}

func TestPluginConfigs(t *testing.T) {
	t.Run("extracts per-plugin tables", func(t *testing.T) {
		cmd := cmdWithConfigFile(t, `
[plugins.metrics]
url = "https://influx.internal:8086"
bucket = "knot"

[plugins.dashboard]
theme = "dark"
retries = 3
verify_tls = false
tags = ["zone", "server"]

[plugins.dashboard.auth]
method = "token"
`)
		configs, warnings := PluginConfigs(cmd)
		if len(warnings) != 0 {
			t.Fatalf("warnings = %v, want none", warnings)
		}
		if len(configs) != 2 {
			t.Fatalf("configs = %v, want two plugins", configs)
		}
		if got := configs["metrics"]["url"]; got != "https://influx.internal:8086" {
			t.Fatalf("metrics url = %v", got)
		}
		dash := configs["dashboard"]
		if dash["retries"] != int64(3) || dash["verify_tls"] != false {
			t.Fatalf("dashboard scalars = %v / %v", dash["retries"], dash["verify_tls"])
		}
		auth, ok := dash["auth"].(map[string]any)
		if !ok || auth["method"] != "token" {
			t.Fatalf("nested table = %v", dash["auth"])
		}
	})

	t.Run("no plugins section yields empty", func(t *testing.T) {
		cmd := cmdWithConfigFile(t, `[server]
listen = ":3000"
`)
		configs, warnings := PluginConfigs(cmd)
		if len(configs) != 0 || len(warnings) != 0 {
			t.Fatalf("configs = %v warnings = %v, want empty", configs, warnings)
		}
	})

	t.Run("non-table entries warn and skip", func(t *testing.T) {
		cmd := cmdWithConfigFile(t, `
[plugins]
simple = "not a table"

[plugins.real]
key = "value"
`)
		configs, warnings := PluginConfigs(cmd)
		if len(configs) != 1 || configs["real"]["key"] != "value" {
			t.Fatalf("configs = %v, want only the real table", configs)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "simple") {
			t.Fatalf("warnings = %v, want one naming simple", warnings)
		}
	})

	t.Run("nil command is safe", func(t *testing.T) {
		configs, warnings := PluginConfigs(nil)
		if len(configs) != 0 || len(warnings) != 0 {
			t.Fatalf("nil command: %v / %v", configs, warnings)
		}
	})
}
