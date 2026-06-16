package service

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func newTestUser(t *testing.T) *model.User {
	t.Helper()
	return &model.User{
		Id:    "user-admin",
		Roles: []string{model.RoleAdminUUID},
	}
}

func newTestSpace(t *testing.T, name string) *model.Space {
	t.Helper()
	altNames := []model.AltNameEntry{}
	space := model.NewSpace(name, "test", "user-admin", "", "bash", &altNames, "", "", nil)
	space.Stack = "mystack"
	space.StackPrefix = "myapp"
	return space
}

// TestUpdateSpaceStackPrefixProves that the server-managed prefix is persisted
// through the update whitelist: it is cleared when the stack name is removed
// (detaching the space) and preserved while the space stays in a stack.
func TestUpdateSpaceStackPrefix(t *testing.T) {
	// Configure an embedded BadgerDB and a full-permission admin role for this
	// test. Setup is done inline (not in TestMain) because other tests in this
	// package set their own config/role caches.
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	model.SetRoleCache(nil)

	db := database.GetInstance()
	user := newTestUser(t)

	t.Run("clearing the stack name clears the prefix", func(t *testing.T) {
		space := newTestSpace(t, "detach-space")
		if err := db.SaveSpace(space, nil); err != nil {
			t.Fatalf("SaveSpace create: %v", err)
		}

		// Reload through the service (ownership/zone checks), detach and clear prefix.
		loaded, err := GetSpaceService().GetSpace(space.Id, user)
		if err != nil {
			t.Fatalf("GetSpace: %v", err)
		}
		loaded.Stack = ""
		loaded.StackPrefix = ""

		if err := GetSpaceService().UpdateSpace(loaded, user); err != nil {
			t.Fatalf("UpdateSpace: %v", err)
		}

		after, err := db.GetSpace(space.Id)
		if err != nil {
			t.Fatalf("db.GetSpace: %v", err)
		}
		if after.Stack != "" {
			t.Errorf("Stack = %q, want empty", after.Stack)
		}
		if after.StackPrefix != "" {
			t.Errorf("StackPrefix = %q, want empty (detach should clear the prefix)", after.StackPrefix)
		}
	})

	t.Run("a set prefix is persisted through update", func(t *testing.T) {
		space := newTestSpace(t, "keep-space")
		if err := db.SaveSpace(space, nil); err != nil {
			t.Fatalf("SaveSpace create: %v", err)
		}

		loaded, err := GetSpaceService().GetSpace(space.Id, user)
		if err != nil {
			t.Fatalf("GetSpace: %v", err)
		}
		loaded.Description = "updated description"
		// StackPrefix is left at its loaded value ("myapp"); staying in the stack
		// should persist it (proves StackPrefix is in the update whitelist).

		if err := GetSpaceService().UpdateSpace(loaded, user); err != nil {
			t.Fatalf("UpdateSpace: %v", err)
		}

		after, err := db.GetSpace(space.Id)
		if err != nil {
			t.Fatalf("db.GetSpace: %v", err)
		}
		if after.Stack != "mystack" {
			t.Errorf("Stack = %q, want %q", after.Stack, "mystack")
		}
		if after.StackPrefix != "myapp" {
			t.Errorf("StackPrefix = %q, want %q", after.StackPrefix, "myapp")
		}
	})
}
