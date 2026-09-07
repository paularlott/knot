package plugins

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLoadExamplePlugins loads the repository's example plugins end to end:
// demo-scriptling must register, and demo-go's state depends on whether its
// Go peer has been built into bin/ (make in examples/plugins/demo-go). When
// the peer is present its handshake satisfies the metadata dependency and
// the plugin loads with a healthy peer; when it is not, the plugin fails
// with the missing-plugin requirement — both are valid outcomes, so the
// test asserts whichever holds.
func TestLoadExamplePlugins(t *testing.T) {
	examples := filepath.Join("..", "..", "examples", "plugins")
	if _, err := os.Stat(examples); err != nil {
		t.Skip("examples/plugins not present")
	}

	registry, err := Load(examples)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()

	var scriptling *Plugin
	for _, p := range registry.All() {
		if p.Name == "demo-scriptling" {
			scriptling = p
		}
	}
	if scriptling == nil {
		t.Fatalf("demo-scriptling not loaded; failed = %+v", registry.Failed())
	}
	if len(scriptling.Permissions) != 2 {
		t.Errorf("permissions = %+v", scriptling.Permissions)
	}
	if scriptling.Permissions[0].Id != "plugin.demo-scriptling.view_dashboard" {
		t.Errorf("first permission = %q", scriptling.Permissions[0].Id)
	}
	if len(scriptling.Menus) != 3 {
		t.Errorf("menus = %+v", scriptling.Menus)
	}
	// demo-scriptling publishes an in-process scriptling library:
	// constants, functions and a class importable as plugin.calc.
	if len(scriptling.Libs) != 1 || scriptling.Libs[0].Name != "calc" {
		t.Errorf("script libs = %+v, want calc", scriptling.Libs)
	}
	// demo-scriptling ships a themed pair, which claims the site logo.
	if !scriptling.SiteLogo {
		t.Error("demo-scriptling's logo pair should claim the site logo")
	}
	if scriptling.LogoLight != "assets/logo-light.svg" || scriptling.LogoDark != "assets/logo-dark.svg" {
		t.Errorf("logos = %q / %q", scriptling.LogoLight, scriptling.LogoDark)
	}
	if len(scriptling.Pages) != 1 || scriptling.Pages[0].Path != "/showcase" || scriptling.Pages[0].Handler != "showcase" {
		t.Errorf("pages = %+v", scriptling.Pages)
	}
	if scriptling.Pages[0].MenuLabel == "" {
		t.Errorf("dashboard page should carry a menu_label: %+v", scriptling.Pages[0])
	}
	// Icons are the plugin's own SVG assets, loaded and sanitized at load.
	for _, menu := range scriptling.Menus {
		if menu.Icon != "assets/icon.svg" || !strings.Contains(menu.IconSVG, "<path") {
			t.Errorf("menu %q icon not loaded: %+v", menu.Label, menu)
		}
	}

	peerBuilt := false
	if _, err := os.Stat(filepath.Join(examples, "demo-go", "bin", "demolib_"+runtime.GOOS+"_"+runtime.GOARCH)); err == nil {
		peerBuilt = true
	}

	var demoGo *Plugin
	for _, p := range registry.All() {
		if p.Name == "demo-go" {
			demoGo = p
		}
	}
	if peerBuilt {
		if demoGo == nil {
			t.Fatalf("demo-go failed to load with peer built: %+v", registry.Failed())
		}
		// demo-go ships a single logo; knot copies it to the dark slot.
		if !demoGo.SiteLogo || demoGo.LogoLight == "" || demoGo.LogoDark != demoGo.LogoLight {
			t.Errorf("demo-go single logo not copied to both slots: %q / %q", demoGo.LogoLight, demoGo.LogoDark)
		}
		peers := registry.Peers(demoGo)
		// A peer declaring the bare name "demolib" is registered under
		// scriptling's host-owned plugin. namespace.
		if len(peers) != 1 || peers[0].Name != "plugin.demolib" || peers[0].Version != "1.0.0" || !peers[0].Healthy {
			t.Errorf("peers = %+v", peers)
		}
	} else {
		if demoGo != nil {
			t.Fatalf("demo-go loaded without its peer; want requirement failure")
		}
		found := false
		for _, failed := range registry.Failed() {
			if failed.Name == "demo-go" && strings.Contains(failed.Reason, "demolib") {
				found = true
			}
		}
		if !found {
			t.Errorf("failed = %+v, want demo-go missing-peer failure", registry.Failed())
		}
	}
}
