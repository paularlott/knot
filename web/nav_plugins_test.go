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
// and restores the (nil) global afterwards.
func withPluginRegistry(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "hello")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := "# /// script\n# [tool.knot]\nversion = \"1.0\"\n# ///\n"
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
