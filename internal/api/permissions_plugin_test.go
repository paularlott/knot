package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
)

// pluginPermissionFixture loads one plugin declaring a permission, and a
// role granting it, so the permission surfaces can be checked end to end.
func pluginPermissionFixture(t *testing.T) *model.User {
	t.Helper()
	rest.SetAPIMux(http.NewServeMux())
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "granter")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `# /// script
# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0"
# permissions = ["read"]
# ///
def unused():
    return {}
`
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

	role := model.NewRole("granters", nil, "u1")
	role.PluginPermissions = []string{"plugin.granter.read"}
	model.SetRoleCache([]*model.Role{role})
	t.Cleanup(func() { model.SetRoleCache(nil) })

	user := &model.User{Username: "kai", Id: "u-kai", Roles: []string{role.Id}}
	db := database.GetInstance()
	if err := db.SaveUser(user, nil); err != nil {
		t.Fatal(err)
	}
	return user
}

// TestPermissionsEndpointsIncludePluginGrants pins that the permission
// surfaces carry plugin-defined permissions alongside the built-ins: the
// catalog lists every loaded plugin's declared grants, and a user's
// resolved permissions include the grants their roles hold.
func TestPermissionsEndpointsIncludePluginGrants(t *testing.T) {
	pluginPermissionFixture(t)

	w := httptest.NewRecorder()
	HandleGetPermissions(w, httptest.NewRequest(http.MethodGet, "/api/permissions", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("permissions catalog status = %d, body = %s", w.Code, w.Body.String())
	}
	var catalog struct {
		Permissions []struct {
			Id int `json:"id"`
		} `json:"permissions"`
		PluginPermissions []struct {
			Id     string `json:"id"`
			Plugin string `json:"plugin"`
			Label  string `json:"label"`
		} `json:"plugin_permissions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Permissions) == 0 {
		t.Error("built-in permissions missing from the catalog")
	}
	if len(catalog.PluginPermissions) != 1 ||
		catalog.PluginPermissions[0].Id != "plugin.granter.read" ||
		catalog.PluginPermissions[0].Plugin != "granter" ||
		catalog.PluginPermissions[0].Label != "read" {
		t.Errorf("plugin permissions in catalog = %+v", catalog.PluginPermissions)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/users/kai/permissions", nil)
	r.SetPathValue("user_id", "kai")
	w = httptest.NewRecorder()
	HandleGetUserPermissions(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("user permissions status = %d, body = %s", w.Code, w.Body.String())
	}
	var perms struct {
		Permissions       []uint16 `json:"permissions"`
		PluginPermissions []string `json:"plugin_permissions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &perms); err != nil {
		t.Fatal(err)
	}
	if len(perms.PluginPermissions) != 1 || perms.PluginPermissions[0] != "plugin.granter.read" {
		t.Errorf("user plugin permissions = %v, want [plugin.granter.read]", perms.PluginPermissions)
	}
}
