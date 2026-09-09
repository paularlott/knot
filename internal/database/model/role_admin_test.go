package model

import "testing"

// TestAdminRoleHoldsAllCorePermissions pins the fixed admin role against
// the permission catalog: every permission ships in the admin role's list,
// so a new permission that forgets the list fails here instead of silently
// hiding its UI from admins. UseLogSinks is the one exception — Core never
// grants or checks it (Pro's copy of this role adds it back).
func TestAdminRoleHoldsAllCorePermissions(t *testing.T) {
	SetRoleCache(nil)
	admin := roleCache[RoleAdminUUID]
	held := make(map[uint16]bool, len(admin.Permissions))
	for _, p := range admin.Permissions {
		held[p] = true
	}
	for _, pn := range PermissionNames {
		if pn.Id == int(PermissionUseLogSinks) {
			continue
		}
		if !held[uint16(pn.Id)] {
			t.Errorf("admin role missing %s (%d)", pn.Name, pn.Id)
		}
	}
}
