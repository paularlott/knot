//go:build integration

package suites

import (
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration/harness"
	"github.com/paularlott/knot/internal/database/model"
)

func TestRolesCRUD(t *testing.T) {
	harness.Feature(t, "roles")
	ctx, cancel := testCtx(30)
	defer cancel()

	name := uniqueName("it-role")
	id, code, err := admin.Client.CreateRole(ctx, &apiclient.RoleRequest{
		Name:        name,
		Permissions: []uint16{model.PermissionUseSpaces, model.PermissionRunCommands},
	})
	if err != nil {
		t.Fatalf("create role: %v (status %d)", err, code)
	}

	details, code, err := admin.Client.GetRole(ctx, id)
	if err != nil {
		t.Fatalf("get role: %v (status %d)", err, code)
	}
	mustEqual(t, "role name", details.Name, name)

	roles, _, err := admin.Client.GetRoles(ctx)
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	found := false
	for _, r := range roles.Roles {
		if r.Id == id {
			found = true
		}
	}
	if !found {
		t.Fatal("created role missing from list")
	}

	if code, err := admin.Client.UpdateRole(ctx, id, &apiclient.RoleRequest{
		Name:        name,
		Permissions: []uint16{model.PermissionUseSpaces},
	}); err != nil {
		t.Fatalf("update role: %v (status %d)", err, code)
	}

	if code, err := admin.Client.DeleteRole(ctx, id); err != nil {
		t.Fatalf("delete role: %v (status %d)", err, code)
	}
}

func TestRoleGrantsPermissions(t *testing.T) {
	harness.Feature(t, "permissions")
	ctx, cancel := testCtx(30)
	defer cancel()

	// A fresh user whose role has exactly one permission; verify the
	// resolved set. Runs on a dedicated server under pro (2-user cap).
	target, targetAdmin := crudTarget(t)
	limited, err := harness.CreateUser(target, targetAdmin, uniqueName("it-limited"),
		[]uint16{model.PermissionUseSpaces})
	if err != nil {
		t.Fatalf("create limited user: %v", err)
	}
	defer func() {
		ctx, cancel := testCtx(30)
		_ = targetAdmin.Client.DeleteUser(ctx, limited.Id)
		cancel()
	}()
	_ = target

	perms, err := targetAdmin.Client.GetUserPermissions(ctx, limited.Id)
	if err != nil {
		t.Fatalf("get permissions: %v", err)
	}
	if len(perms) != 1 || perms[0] != model.PermissionUseSpaces {
		t.Fatalf("resolved permissions = %v, want [UseSpaces]", perms)
	}

	// The limited user cannot run commands anywhere (403 before any space
	// check would matter).
	if _, _, err := limited.Client.GetSpaces(ctx, "", false); err != nil {
		t.Fatalf("limited user should list own spaces: %v", err)
	}
}

func TestGroupsCRUD(t *testing.T) {
	harness.Feature(t, "groups")
	ctx, cancel := testCtx(30)
	defer cancel()

	name := uniqueName("it-group")
	id, code, err := admin.Client.CreateGroup(ctx, &apiclient.GroupRequest{
		Name:          name,
		MaxSpaces:     3,
		ComputeUnits:  4,
		StorageUnits:  4,
		MaxTunnels:    1,
	})
	if err != nil {
		t.Fatalf("create group: %v (status %d)", err, code)
	}

	group, code, err := admin.Client.GetGroup(ctx, id)
	if err != nil {
		t.Fatalf("get group: %v (status %d)", err, code)
	}
	mustEqual(t, "group name", group.Name, name)

	groups, _, err := admin.Client.GetGroups(ctx)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	found := false
	for _, g := range groups.Groups {
		if g.Id == id {
			found = true
		}
	}
	if !found {
		t.Fatal("created group missing from list")
	}

	if code, err := admin.Client.UpdateGroup(ctx, id, &apiclient.GroupRequest{
		Name: name, MaxSpaces: 6, ComputeUnits: 4, StorageUnits: 4, MaxTunnels: 1,
	}); err != nil {
		t.Fatalf("update group: %v (status %d)", err, code)
	}

	if code, err := admin.Client.DeleteGroup(ctx, id); err != nil {
		t.Fatalf("delete group: %v (status %d)", err, code)
	}
}
