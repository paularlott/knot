package model

import (
	"testing"
)

func TestNewRole(t *testing.T) {
	permissions := []uint16{PermissionManageUsers, PermissionUseSpaces}
	role := NewRole("test-role", permissions, "user-123")

	if role.Id == "" {
		t.Error("Role ID should not be empty")
	}
	if role.Name != "test-role" {
		t.Errorf("Expected name 'test-role', got '%s'", role.Name)
	}
	if len(role.Permissions) != 2 {
		t.Errorf("Expected 2 permissions, got %d", len(role.Permissions))
	}
	if role.CreatedUserId != "user-123" {
		t.Errorf("Expected created user ID 'user-123', got '%s'", role.CreatedUserId)
	}
}

func TestRoleCache(t *testing.T) {
	role1 := NewRole("role1", []uint16{PermissionManageUsers}, "user-1")
	role2 := NewRole("role2", []uint16{PermissionUseSpaces}, "user-2")

	SetRoleCache([]*Role{role1, role2})

	if !RoleExists(role1.Id) {
		t.Error("Role1 should exist in cache")
	}
	if !RoleExists(role2.Id) {
		t.Error("Role2 should exist in cache")
	}
	if !RoleExists(RoleAdminUUID) {
		t.Error("Admin role should exist in cache")
	}

	roles := GetRolesFromCache()
	if len(roles) < 3 {
		t.Errorf("Expected at least 3 roles in cache, got %d", len(roles))
	}

	DeleteRoleFromCache(role1.Id)
	if RoleExists(role1.Id) {
		t.Error("Role1 should not exist after deletion")
	}

	role3 := NewRole("role3", []uint16{PermissionManageTemplates}, "user-3")
	SaveRoleToCache(role3)
	if !RoleExists(role3.Id) {
		t.Error("Role3 should exist after save")
	}
}
