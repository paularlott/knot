package model

import (
	"testing"
)

func TestNewGroup(t *testing.T) {
	group := NewGroup("test-group", "user-123", 10, 500, 1000, 5)

	if group.Id == "" {
		t.Error("Group ID should not be empty")
	}
	if group.Name != "test-group" {
		t.Errorf("Expected name 'test-group', got '%s'", group.Name)
	}
	if group.CreatedUserId != "user-123" {
		t.Errorf("Expected created user ID 'user-123', got '%s'", group.CreatedUserId)
	}
	if group.UpdatedUserId != "user-123" {
		t.Errorf("Expected updated user ID 'user-123', got '%s'", group.UpdatedUserId)
	}
	if group.MaxSpaces != 10 {
		t.Errorf("Expected max spaces 10, got %d", group.MaxSpaces)
	}
	if group.ComputeUnits != 500 {
		t.Errorf("Expected compute units 500, got %d", group.ComputeUnits)
	}
	if group.StorageUnits != 1000 {
		t.Errorf("Expected storage units 1000, got %d", group.StorageUnits)
	}
	if group.MaxTunnels != 5 {
		t.Errorf("Expected max tunnels 5, got %d", group.MaxTunnels)
	}
	if group.IsDeleted {
		t.Error("New group should not be deleted")
	}
}
