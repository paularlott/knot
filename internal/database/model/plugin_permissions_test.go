package model

import "testing"

func TestHasPluginPermission(t *testing.T) {
	role := NewRole("plugin-users", nil, "user-1")
	role.PluginPermissions = []string{"plugin.metrics.read", "plugin.metrics.export"}
	SetRoleCache([]*Role{role})
	defer SetRoleCache(nil)

	granted := &User{Username: "granted", Roles: []string{role.Id}}
	other := &User{Username: "other", Roles: []string{}}

	if !granted.HasPluginPermission("plugin.metrics.read") {
		t.Error("granted user should carry the plugin grant")
	}
	if granted.HasPluginPermission("plugin.metrics.admin") {
		t.Error("grant must be exact, not prefix-matched")
	}
	if other.HasPluginPermission("plugin.metrics.read") {
		t.Error("user without the role must not carry the grant")
	}
	if other.HasPluginPermission("garbage") {
		t.Error("malformed grant name should never match")
	}

	// The fixed admin role cannot be granted plugin permissions through the
	// API, so it passes every plugin permission check instead.
	admin := &User{Username: "admin", Roles: []string{RoleAdminUUID}}
	if !admin.HasPluginPermission("plugin.anything.else") {
		t.Error("admin should pass every plugin permission check")
	}
}
