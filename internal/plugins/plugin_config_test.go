package plugins

import (
	"strings"
	"testing"
)

// metadataWithConfig builds a minimal manifest declaring required config
// keys via [tool.knot] config.
func metadataWithConfig(keys ...string) string {
	body := `# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0.0"
# description = "test plugin"`
	if len(keys) > 0 {
		quoted := make([]string, 0, len(keys))
		for _, key := range keys {
			quoted = append(quoted, `"`+key+`"`)
		}
		body += "\n# config = [" + strings.Join(quoted, ", ") + "]"
	}
	return body
}

func loadWith(t *testing.T, metadata string, configs map[string]map[string]any) (*Registry, *Plugin) {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "plug", metadata)
	registry, err := LoadWithConfigs(dir, configs)
	if err != nil {
		t.Fatalf("LoadWithConfigs: %v", err)
	}
	return registry, registry.ByName("plug")
}

func TestPluginConfigAttached(t *testing.T) {
	configs := map[string]map[string]any{
		"plug": {"url": "https://example.com", "retries": 3},
	}
	registry, p := loadWith(t, metadataWithConfig("url", "retries"), configs)
	if p == nil {
		t.Fatal("plugin missing after load")
	}
	if got, ok := p.Config["url"].(string); !ok || got != "https://example.com" {
		t.Fatalf("config url = %v, want https://example.com", p.Config["url"])
	}
	if n := registry.Failed(); len(n) != 0 {
		t.Fatalf("plugin failed: %+v", n)
	}
	if w := registry.Warnings(); len(w) != 0 {
		t.Fatalf("unexpected warnings: %v", w)
	}
}

func TestPluginConfigEmptyWithoutDeclaration(t *testing.T) {
	// A plugin that declares nothing and is configured with nothing loads
	// with an empty (nil) config — handlers see request["config"] = {}.
	_, p := loadWith(t, metadataWithConfig(), nil)
	if p == nil {
		t.Fatal("plugin missing after load")
	}
	if len(p.Config) != 0 {
		t.Fatalf("config = %v, want empty", p.Config)
	}
}

func TestPluginConfigExtraKeysAllowed(t *testing.T) {
	// Declared keys are required, not exhaustive: extra keys ride along.
	_, p := loadWith(t, metadataWithConfig("url"), map[string]map[string]any{
		"plug": {"url": "https://x", "extra": true},
	})
	if p == nil || p.Config["extra"] != true {
		t.Fatalf("extra key rejected: %+v", p)
	}
}

func TestPluginConfigRequiredKeysMissing(t *testing.T) {
	registry, p := loadWith(t, metadataWithConfig("url", "token"), map[string]map[string]any{
		"plug": {"url": "https://x"},
	})
	if p != nil {
		t.Fatalf("plugin loaded without required key: %+v", p)
	}
	failed := registry.Failed()
	if len(failed) != 1 || !strings.Contains(failed[0].Reason, "token") {
		t.Fatalf("failed = %+v, want a reason naming the missing token key", failed)
	}
}

func TestPluginConfigRequiredButUnconfigured(t *testing.T) {
	// Declaring required keys with no [plugins.<name>] section at all is
	// the same failure — the keys are absent.
	registry, p := loadWith(t, metadataWithConfig("url"), nil)
	if p != nil {
		t.Fatalf("plugin loaded without any configuration: %+v", p)
	}
	if failed := registry.Failed(); len(failed) != 1 {
		t.Fatalf("failed = %+v, want one entry", failed)
	}
}

func TestPluginConfigSizeCap(t *testing.T) {
	big := strings.Repeat("x", maxPluginConfigBytes)
	registry, p := loadWith(t, metadataWithConfig(), map[string]map[string]any{
		"plug": {"blob": big},
	})
	if p != nil {
		t.Fatalf("oversized config accepted: %+v", p)
	}
	failed := registry.Failed()
	if len(failed) != 1 || !strings.Contains(failed[0].Reason, "cap") {
		t.Fatalf("failed = %+v, want a size-cap reason", failed)
	}
}

func TestPluginConfigOrphanWarning(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "plug", metadataWithConfig())
	registry, err := LoadWithConfigs(dir, map[string]map[string]any{
		"ghost": {"url": "https://x"},
	})
	if err != nil {
		t.Fatalf("LoadWithConfigs: %v", err)
	}
	if registry.ByName("plug") == nil {
		t.Fatal("plug should load")
	}
	warnings := registry.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "ghost") {
		t.Fatalf("warnings = %v, want a ghost orphan warning", warnings)
	}
}

func TestPluginConfigFailedPluginIsNotAnOrphan(t *testing.T) {
	// A plugin that loaded but failed config validation consumed its
	// section: the failure reason already names it, so no orphan warning.
	registry, p := loadWith(t, metadataWithConfig("missing-key"), map[string]map[string]any{
		"plug": {},
	})
	if p != nil {
		t.Fatal("plugin should fail validation")
	}
	for _, warning := range registry.Warnings() {
		if strings.Contains(warning, "plug") {
			t.Fatalf("validation-failed plugin double-reported as orphan: %s", warning)
		}
	}
}

func TestParseToolKnotConfigDeclaration(t *testing.T) {
	cases := []struct {
		name        string
		decl        string
		wantErr     string
		wantReqired []string
	}{
		{"key list", `config = ["a", "b"]`, "", []string{"a", "b"}},
		{"not a list", `config = "a"`, "config must be a list", nil},
		{"empty key name", `config = [""]`, "non-empty", nil},
		{"non-string entry", `config = [1]`, "non-empty", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			body := "# requires-scriptling = \">=0.1\"\n#\n# [tool.knot]\n# version = \"1.0.0\"\n# " + c.decl
			writePlugin(t, dir, "p", body)
			// The happy path supplies every declared key — a declared
			// plugin with no configuration fails validation by design.
			configs := map[string]map[string]any{"p": {"a": 1, "b": 2}}
			registry, err := LoadWithConfigs(dir, configs)
			if err != nil {
				t.Fatalf("LoadWithConfigs: %v", err)
			}
			if c.wantErr != "" {
				failed := registry.Failed()
				if len(failed) != 1 || !strings.Contains(failed[0].Reason, c.wantErr) {
					t.Fatalf("failed = %+v, want reason containing %q", failed, c.wantErr)
				}
				return
			}
			p := registry.ByName("p")
			if p == nil {
				t.Fatal("plugin missing")
			}
			if len(p.RequiredConfig) != len(c.wantReqired) {
				t.Fatalf("required = %v, want %v", p.RequiredConfig, c.wantReqired)
			}
		})
	}
}
