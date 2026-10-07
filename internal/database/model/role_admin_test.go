package model

import "testing"

// TestAdminRoleHoldsAllCorePermissions pins the fixed admin role against
// the permission catalog: every permission ships in the admin role's list,
// so a new permission that forgets the list fails here instead of silently
// hiding its UI from admins. UseLogSinks is one exception — Core never
// grants or checks it (Pro's copy of this role adds it back) — and Backup is
// the other: it is held by the Backup User role, never by admins.
func TestAdminRoleHoldsAllCorePermissions(t *testing.T) {
	SetRoleCache(nil)
	admin := roleCache[RoleAdminUUID]
	held := make(map[uint16]bool, len(admin.Permissions))
	for _, p := range admin.Permissions {
		held[p] = true
	}
	for _, pn := range PermissionNames {
		if pn.Id == int(PermissionUseLogSinks) || pn.Id == int(PermissionBackup) {
			continue
		}
		if !held[uint16(pn.Id)] {
			t.Errorf("admin role missing %s (%d)", pn.Name, pn.Id)
		}
	}
}

// Backup is a separate grant: admins lack it, and no role holds it until one
// is made, as a new server does for its first user.
func TestBackupPermissionIsSeparate(t *testing.T) {
	SetRoleCache(nil)
	for _, p := range roleCache[RoleAdminUUID].Permissions {
		if p == PermissionBackup {
			t.Error("the admin role holds the backup permission")
		}
	}
	for _, r := range GetRolesFromCache() {
		for _, p := range r.Permissions {
			if p == PermissionBackup {
				t.Errorf("an existing system has a role %q holding the backup permission it did not create", r.Name)
			}
		}
	}

	backup := NewRole(BackupRoleName, []uint16{PermissionBackup}, "")
	SaveRoleToCache(backup)
	admin := &User{Roles: []string{RoleAdminUUID}}
	both := &User{Roles: []string{RoleAdminUUID, backup.Id}}
	if admin.HasPermission(PermissionBackup) || !both.HasPermission(PermissionBackup) || !both.HasPermission(PermissionManageUsers) {
		t.Error("permissions follow the roles")
	}
	if !IsBuiltinRole(RoleAdminUUID) || IsBuiltinRole(backup.Id) {
		t.Error("only the admin role is built in: the backup role can be edited and removed")
	}
	DeleteRoleFromCache(backup.Id)
}
