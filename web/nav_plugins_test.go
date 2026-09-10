package web

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
)

// withPluginRegistry loads a one-plugin registry for the duration of the test
// and restores the (nil) global afterwards. The plugin is named "hello" after
// its directory, so a declared page path "/dashboard" is served at
// /plugins/hello/dashboard.
func withPluginRegistry(t *testing.T) {
	withPluginRegistrySource(t, "# /// script\n# [tool.knot]\nversion = \"1.0\"\n# ///\n")
}

func withPluginRegistrySource(t *testing.T, source string) {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "hello")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := plugins.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	plugins.SetRegistry(registry)
	t.Cleanup(func() {
		registry.Close()
		plugins.SetRegistry(nil)
	})
}

// TestNavPluginsItemPresence pins the "no plugins, no trace" rule: the admin
// inventory item appears only when the plugins path produced anything.
func TestNavPluginsItemPresence(t *testing.T) {
	model.SetRoleCache(nil)
	cfg := &config.ServerConfig{}
	admin := &model.User{Username: "admin", Roles: []string{model.RoleAdminUUID}}

	// No registry (plugins path unset): nothing anywhere.
	plugins.SetRegistry(nil)
	_, more := buildNav(admin, cfg, true)
	for _, item := range more {
		if item.URL == "/plugins" {
			t.Error("plugins inventory item must not appear with no plugins loaded")
		}
	}

	// Registry present: the item appears under Cluster Info for admins.
	cfg.Cluster.AdvertiseAddr = "127.0.0.1:9000"
	withPluginRegistry(t)
	_, more = buildNav(admin, cfg, true)
	found, clusterIdx, pluginsIdx := false, -1, -1
	for i, item := range more {
		switch item.URL {
		case "/cluster-info":
			clusterIdx = i
		case "/plugins":
			found, pluginsIdx = true, i
		}
	}
	if !found {
		t.Fatalf("plugins inventory item missing with plugins loaded: %v", urls(more))
	}
	if clusterIdx >= 0 && pluginsIdx < clusterIdx {
		t.Errorf("plugins item (%d) should sit under Cluster Info (%d)", pluginsIdx, clusterIdx)
	}

	// A non-admin never sees it.
	plain := &model.User{Username: "plain"}
	_, more = buildNav(plain, cfg, true)
	for _, item := range more {
		if item.URL == "/plugins" {
			t.Error("plugins inventory item must not appear for non-admins")
		}
	}
}

// TestNavPluginPagePinKeepsMoreClosed pins the regression where a plugin page
// (served under /plugins/<name>) also prefix-matched the shorter /plugins
// inventory entry inside More, so More auto-expanded on every plugin page
// even with the plugin's own menu item pinned out of it.
func TestNavPluginPagePinKeepsMoreClosed(t *testing.T) {
	model.SetRoleCache(nil)
	cfg := &config.ServerConfig{}
	withPluginRegistrySource(t, "# /// script\n"+
		"# [tool.knot]\n"+
		"# version = \"1.0\"\n"+
		"#\n"+
		"# [[tool.knot.pages]]\n"+
		"# path = \"/dashboard\"\n"+
		"# handler = \"dashboard\"\n"+
		"# menu_label = \"Dashboard\"\n"+
		"# ///\n"+
		"\n"+
		"def dashboard(req):\n"+
		"    return \"ok\"\n")
	admin := &model.User{Username: "admin", Roles: []string{model.RoleAdminUUID}}
	const pageURL = "/plugins/hello/dashboard"

	// Sanity: the page's menu item is a visible More entry alongside the
	// inventory item it nests under.
	_, more := buildNav(admin, cfg, true)
	hasPage, hasInventory := false, false
	for _, item := range more {
		hasPage = hasPage || item.URL == pageURL
		hasInventory = hasInventory || item.URL == "/plugins"
	}
	if !hasPage || !hasInventory {
		t.Fatalf("want both %s and /plugins in More, got %v", pageURL, urls(more))
	}

	// Unpinned (Mode A) on the plugin page: More opens to reveal the active
	// item.
	if _, _, _, _, moreActive := resolveNav(admin, cfg, true, pageURL); !moreActive {
		t.Fatal("Mode A on a plugin page: More should auto-expand to reveal it")
	}

	// Pinned (Mode B) on the plugin page: the page's own item owns the path,
	// not the shorter /plugins entry, so More must stay closed.
	admin.SetNavStarred([]string{pageURL})
	if _, _, _, _, moreActive := resolveNav(admin, cfg, true, pageURL); moreActive {
		t.Fatal("Mode B with the plugin page pinned: More must stay closed")
	}

	// The inventory page itself still lives in More and opens it.
	if _, _, _, _, moreActive := resolveNav(admin, cfg, true, "/plugins"); !moreActive {
		t.Fatal("the /plugins inventory page lives in More: it should auto-expand")
	}
}
