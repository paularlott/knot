package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// iconPluginDir builds a folder plugin whose single menu uses the given icon
// file content, and returns the plugins root.
func iconPluginDir(t *testing.T, iconContent string) string {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "iconed")
	assets := filepath.Join(pluginDir, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "icon.svg"), []byte(iconContent), 0o644); err != nil {
		t.Fatal(err)
	}
	writePlugin(t, dir, "iconed", `# [tool.knot]
version = "1.0"

[[tool.knot.menus]]
label = "L"
url = "/x"
icon = "assets/icon.svg"`)
	return dir
}

func TestMenuIconAsset(t *testing.T) {
	good := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4 4h16v16H4z"/></svg>`

	registry, err := Load(iconPluginDir(t, good))
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if len(registry.Failed()) != 0 {
		t.Fatalf("failed = %+v", registry.Failed())
	}
	menu := registry.All()[0].Menus[0]
	if menu.Icon != "assets/icon.svg" {
		t.Errorf("icon = %q", menu.Icon)
	}
	// The inner markup is extracted; knot wraps it in the site's svg attrs.
	if !strings.Contains(menu.IconSVG, "<path") || strings.Contains(menu.IconSVG, "<svg") {
		t.Errorf("IconSVG = %q, want inner markup only", menu.IconSVG)
	}

	for _, tc := range []struct {
		name  string
		icon  string
		match string
	}{
		{"script", `<svg><path d="M1 1"/><script>alert(1)</script></svg>`, "not allowed"},
		{"event handler", `<svg><path onclick="alert(1)" d="M1 1"/></svg>`, "not allowed"},
		{"external ref", `<svg><use href="#x"/></svg>`, "not allowed"},
		{"not svg", `<div>hello</div>`, "not a valid SVG"},
		{"empty", `<svg></svg>`, "no content"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry, err := Load(iconPluginDir(t, tc.icon))
			if err != nil {
				t.Fatal(err)
			}
			defer registry.Close()
			if len(registry.Failed()) != 1 || !strings.Contains(registry.Failed()[0].Reason, tc.match) {
				t.Fatalf("failed = %+v, want %q", registry.Failed(), tc.match)
			}
		})
	}
}
