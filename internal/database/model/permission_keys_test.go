package model

import "testing"

// TestPermissionKeysComplete pins the invariant the user surface relies
// on: every permission listed for the role editor carries a stable
// snake_case key, so user.has_permission never keys on display names and
// a new permission without a key fails here rather than silently
// vanishing from plugins' view.
func TestPermissionKeysComplete(t *testing.T) {
	for _, pn := range PermissionNames {
		if permissionKeys[uint16(pn.Id)] == "" {
			t.Errorf("permission %q (id %d) has no stable key", pn.Name, pn.Id)
		}
	}
	if permissionKeys[PermissionManageSpaces] != "manage_spaces" {
		t.Errorf("manage_spaces key = %q", permissionKeys[PermissionManageSpaces])
	}
	if permissionKeys[PermissionUseMCPServer] != "use_mcp_server" {
		t.Errorf("use_mcp_server key = %q", permissionKeys[PermissionUseMCPServer])
	}
}

// TestGrantedPermissionKeysAreStable pins that the user surface exposes
// keys, not display names.
func TestGrantedPermissionKeysAreStable(t *testing.T) {
	SetRoleCache(nil)
	admin := &User{Username: "a", Roles: []string{RoleAdminUUID}}
	keys := admin.GrantedPermissionKeys()
	if len(keys) == 0 {
		t.Fatal("admin granted no permission keys")
	}
	for _, k := range keys {
		for _, c := range k {
			if c == ' ' || (c >= 'A' && c <= 'Z') {
				t.Fatalf("key %q is not snake_case (display name leaked?)", k)
			}
		}
	}
}
