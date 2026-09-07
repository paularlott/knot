package plugins

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

// TestVisibleMenusGating exercises the single visibility gate shared by the
// sidebar, pin validation, and search: permission-gated and group-gated
// items appear only for users who pass the gate.
func TestVisibleMenusGating(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "gated", `# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0"
# permissions = ["read", "admin"]
#
# [[tool.knot.menus]]
# label = "Public"
# url = "https://example.com/public"
#
# [[tool.knot.menus]]
# label = "Readers"
# url = "https://example.com/read"
# permission = "read"
#
# [[tool.knot.menus]]
# label = "Admins"
# url = "https://example.com/admin"
# permission = "admin"
#`)

	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()

	grantedRole := model.NewRole("granted", nil, "u1")
	grantedRole.PluginPermissions = []string{"plugin.gated.read"}
	platformRole := model.NewRole("platform", nil, "u2")
	model.SetRoleCache([]*model.Role{grantedRole, platformRole})
	defer model.SetRoleCache(nil)

	plain := &model.User{Username: "plain", Roles: []string{platformRole.Id}}
	reader := &model.User{Username: "reader", Roles: []string{grantedRole.Id}}
	admin := &model.User{Username: "admin", Roles: []string{model.RoleAdminUUID}}

	// Permission gate: plain users see only the public item; the reader also
	// sees the permission-gated one; admin sees everything.
	if got := registry.VisibleMenuURLs(plain); len(got) != 1 || !got["https://example.com/public"] {
		t.Errorf("plain URLs = %v", got)
	}
	if got := registry.VisibleMenuURLs(reader); len(got) != 2 || !got["https://example.com/read"] {
		t.Errorf("reader URLs = %v", got)
	}
	if got := registry.VisibleMenuURLs(admin); len(got) != 3 {
		t.Errorf("admin URLs = %v, want all three", got)
	}

	// The pin-validation question: exactly the visible URLs are pinnable.
	for _, tc := range []struct {
		user *model.User
		url  string
		want bool
	}{
		{plain, "https://example.com/public", true},
		{plain, "https://example.com/read", false},
		{reader, "https://example.com/read", true},
		{reader, "https://example.com/not-declared", false},
	} {
		if got := registry.VisibleMenuURLs(tc.user)[tc.url]; got != tc.want {
			t.Errorf("pinnable(%s, %s) = %v, want %v", tc.user.Username, tc.url, got, tc.want)
		}
	}

	// Menus carry their owning plugin.
	for _, menu := range registry.VisibleMenus(admin) {
		if menu.PluginName != "gated" {
			t.Errorf("menu %q PluginName = %q", menu.Label, menu.PluginName)
		}
	}
}
