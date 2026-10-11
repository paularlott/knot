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
	more := allItems(buildNav(admin, cfg, true))
	for _, item := range more {
		if item.URL == "/plugins" {
			t.Error("plugins inventory item must not appear with no plugins loaded")
		}
	}

	// Registry present: the item appears under Cluster Info for admins.
	cfg.Cluster.AdvertiseAddr = "127.0.0.1:9000"
	withPluginRegistry(t)
	more = section(buildNav(admin, cfg, true), navAdmin)
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
	more = allItems(buildNav(plain, cfg, true))
	for _, item := range more {
		if item.URL == "/plugins" {
			t.Error("plugins inventory item must not appear for non-admins")
		}
	}
}

// TestNavPluginPageOwnsItsPath pins the regression where a plugin page
// (served under /plugins/<name>) also prefix-matched the shorter /plugins
// inventory entry, opening the wrong section.
func TestNavPluginPageOwnsItsPath(t *testing.T) {
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

	// Sanity: the page's menu item is in Extensions, the inventory item it
	// nests under in Admin.
	sections := buildNav(admin, cfg, true)
	hasPage, hasInventory := false, false
	for _, item := range section(sections, navPlugins) {
		hasPage = hasPage || item.URL == pageURL
	}
	for _, item := range section(sections, navAdmin) {
		hasInventory = hasInventory || item.URL == "/plugins"
	}
	if !hasPage || !hasInventory {
		t.Fatalf("want %s in Extensions and /plugins in Admin, got %v", pageURL, urls(allItems(sections)))
	}

	// On the plugin page its own item owns the path, not the shorter
	// /plugins entry: Extensions opens, Admin stays closed.
	_, sections = resolveNav(admin, cfg, true, pageURL)
	if !sectionByKey(sections, navPlugins).Active || sectionByKey(sections, navAdmin).Active {
		t.Fatal("a plugin page should open Extensions only")
	}

	// Starred, the page leaves Extensions (now empty and hidden) and no
	// section opens.
	admin.SetNavStarred([]string{pageURL})
	starred, sections := resolveNav(admin, cfg, true, pageURL)
	if len(starred) != 1 || !starred[0].Active {
		t.Fatalf("want the starred plugin page current, got %v", starred)
	}
	for _, s := range sections {
		if s.Active {
			t.Fatalf("section %s opened for a starred page", s.Key)
		}
		if s.Key == navPlugins {
			t.Fatal("Extensions should be hidden once its only item is starred")
		}
	}

	// The inventory page itself lives in Admin and opens it.
	if _, sections := resolveNav(admin, cfg, true, "/plugins"); !sectionByKey(sections, navAdmin).Active {
		t.Fatal("the /plugins inventory page lives in Admin: it should open")
	}
}
