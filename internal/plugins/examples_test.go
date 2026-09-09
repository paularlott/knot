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
// test asserts whichever holds. demo-scriptlingcli2 (the pure scriptling
// CLI peer) is likewise load-gated on the scriptling CLI being on PATH.
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
	// Its row-action icons are its own declared assets, sanitized at load.
	if len(scriptling.ActionIcons) != 7 || !strings.Contains(scriptling.ActionIcons["assets/archive.svg"], "<path") {
		t.Errorf("demo-scriptling action icons = %+v", scriptling.ActionIcons)
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
		// Without the built peer demo-go has no metadata source (it ships
		// no main.py — everything comes from the peer handshake), so it is
		// simply absent: neither loaded nor failed.
		if demoGo != nil {
			t.Fatalf("demo-go loaded without its peer; want it absent")
		}
	}

	// demo-scriptlingcli2 is the pure scriptling-CLI peer: its handshake
	// metadata is the plugin's only manifest (the folder ships no main.py).
	// With the CLI on PATH the plugin loads from that manifest; without it
	// the peer cannot spawn, so the plugin is named as failed on the admin
	// Plugins page. Both are valid, so the test asserts whichever holds.
	var cli2 *Plugin
	for _, p := range registry.All() {
		if p.Name == "demo-scriptlingcli2" {
			cli2 = p
		}
	}
	if cli2 != nil {
		if len(cli2.Permissions) != 1 || cli2.Permissions[0].Id != "plugin.demo-scriptlingcli2.use_notes" {
			t.Errorf("demo-scriptlingcli2 permissions = %+v", cli2.Permissions)
		}
		if len(cli2.Pages) != 1 || cli2.Pages[0].Path != "/notes" || cli2.Pages[0].Handler != "notes_page" {
			t.Errorf("demo-scriptlingcli2 pages = %+v", cli2.Pages)
		}
		if cli2.Pages[0].Icon != "assets/icon.svg" || !strings.Contains(cli2.Pages[0].IconSVG, "<path") {
			t.Errorf("demo-scriptlingcli2 page icon not loaded: %+v", cli2.Pages[0])
		}
		// Declared action-icon assets are sanitized at load and addressable
		// by path from data-driven row actions.
		if !strings.Contains(cli2.ActionIcons["assets/view.svg"], "<path") || !strings.Contains(cli2.ActionIcons["assets/delete.svg"], "<path") {
			t.Errorf("demo-scriptlingcli2 action icons = %+v", cli2.ActionIcons)
		}
		if cli2.EntryFile != "" {
			t.Errorf("demo-scriptlingcli2 must have no entry file, got %q", cli2.EntryFile)
		}
		// With no main.py, the sole peer's handshake name is the namespace
		// bare handler declarations resolve under.
		if ns := cli2.DefaultNamespace(); ns != "notes" {
			t.Errorf("demo-scriptlingcli2 default namespace = %q, want notes", ns)
		}
	} else {
		failed := false
		for _, f := range registry.Failed() {
			if f.Name == "demo-scriptlingcli2" {
				failed = true
			}
		}
		if !failed {
			t.Fatalf("demo-scriptlingcli2 neither loaded nor failed; failed = %+v", registry.Failed())
		}
	}
}
