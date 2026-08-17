//go:build integration

package suites

import (
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration/harness"
)

func TestUsersCRUD(t *testing.T) {
	harness.Feature(t, "users")
	name := uniqueName("it-user")
	ctx, cancel := testCtx(30)
	defer cancel()

	req := &apiclient.CreateUserRequest{
		Username:       name,
		Password:       "Passw0rd!crud",
		Email:          name + "@knot.test",
		Roles:          []string{},
		Active:         true,
		MaxSpaces:      5,
		ComputeUnits:   5,
		StorageUnits:   5,
		MaxTunnels:     2,
		PreferredShell: "bash",
		Timezone:       "UTC",
	}
	userId, code, err := admin.Client.CreateUser(ctx, req)
	if err != nil {
		t.Fatalf("create user: %v (status %d)", err, code)
	}

	user, err := admin.Client.GetUser(ctx, userId)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	mustEqual(t, "username", user.Username, name)
	mustEqual(t, "max spaces", int(user.MaxSpaces), 5)

	// Update.
	user.MaxSpaces = 9
	upd := &apiclient.UpdateUserRequest{
		Username:       user.Username,
		Email:          user.Email,
		Roles:          user.Roles,
		Groups:         user.Groups,
		Active:         user.Active,
		MaxSpaces:      9,
		ComputeUnits:   5,
		StorageUnits:   5,
		MaxTunnels:     2,
		PreferredShell: "bash",
		Timezone:       "UTC",
	}
	if err := admin.Client.UpdateUser(ctx, userId, upd); err != nil {
		t.Fatalf("update user: %v", err)
	}
	user, err = admin.Client.GetUser(ctx, userId)
	if err != nil {
		t.Fatalf("get user after update: %v", err)
	}
	mustEqual(t, "max spaces after update", int(user.MaxSpaces), 9)

	// Duplicate username is rejected.
	if _, code, err := admin.Client.CreateUser(ctx, req); err == nil {
		t.Fatal("duplicate user created")
	} else if code != 400 {
		t.Fatalf("duplicate user status = %d, want 400", code)
	}

	// List contains the user.
	users, err := admin.Client.GetUsers(ctx, "", "")
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	found := false
	for _, u := range users.Users {
		if u.Id == userId {
			found = true
		}
	}
	if !found {
		t.Fatal("created user missing from list")
	}

	if err := admin.Client.DeleteUser(ctx, userId); err != nil {
		t.Fatalf("delete user: %v", err)
	}
}

func TestUserPermissionsEnforced(t *testing.T) {
	harness.Feature(t, "permissions")
	ctx, cancel := testCtx(30)
	defer cancel()

	// user1 has no user-management permission.
	_, code, err := user1.Client.CreateUser(ctx, &apiclient.CreateUserRequest{
		Username:       uniqueName("it-nope"),
		Password:       "Passw0rd!nope",
		Email:          "nope@knot.test",
		Active:         true,
		MaxSpaces:      1,
		ComputeUnits:   1,
		StorageUnits:   1,
		MaxTunnels:     1,
		PreferredShell: "bash",
		Timezone:       "UTC",
	})
	if err == nil {
		t.Fatal("user1 created a user without permission")
	}
	mustEqual(t, "forbidden status", code, 403)

	// Reading templates is allowed for all users (the space form needs the
	// picker); writing them is not.
	if _, _, err := user1.Client.GetTemplates(ctx); err != nil {
		t.Fatalf("user1 should be able to list templates: %v", err)
	}
	if _, code, err := user1.Client.CreateTemplate(ctx, &apiclient.TemplateCreateRequest{
		Name: uniqueName("it-nope"), Platform: "container", Active: true,
		Job: "image: paularlott/knot-ubuntu:26.04", MaxUptimeUnit: "disabled",
	}); err == nil {
		t.Fatal("user1 created a template without permission")
	} else if code != 403 {
		t.Fatalf("template create status = %d, want 403", code)
	}
}

func TestUserQuotaMaxSpaces(t *testing.T) {
	harness.Feature(t, "quotas")
	ctx, cancel := testCtx(120)
	defer cancel()

	// Apply the quota to user1 (this runs before the workhorse exists, so
	// user1 starts at zero spaces; on pro the 2-user cap prevents creating
	// a dedicated quota user).
	info, err := admin.Client.GetUser(ctx, user1.Id)
	if err != nil {
		t.Fatalf("get user1: %v", err)
	}
	info.MaxSpaces = 1
	if err := admin.Client.UpdateUser(ctx, user1.Id, &apiclient.UpdateUserRequest{
		Username:       info.Username,
		Email:          info.Email,
		Password:       "Passw0rd!test",
		Roles:          info.Roles,
		Groups:         info.Groups,
		Active:         true,
		MaxSpaces:      1,
		ComputeUnits:   20,
		StorageUnits:   20,
		MaxTunnels:     10,
		PreferredShell: "bash",
		Timezone:       "UTC",
	}); err != nil {
		t.Fatalf("shrink quota: %v", err)
	}
	// Restore the quota whatever happens.
	t.Cleanup(func() {
		rctx, rcancel := testCtx(30)
		rinfo, err := admin.Client.GetUser(rctx, user1.Id)
		if err == nil {
			admin.Client.UpdateUser(rctx, user1.Id, &apiclient.UpdateUserRequest{
				Username:       rinfo.Username,
				Email:          rinfo.Email,
				Roles:          rinfo.Roles,
				Groups:         rinfo.Groups,
				Active:         true,
				MaxSpaces:      20,
				ComputeUnits:   20,
				StorageUnits:   20,
				MaxTunnels:     10,
				PreferredShell: "bash",
				Timezone:       "UTC",
			})
		}
		rcancel()
	})

	first := harness.CreateSpace(t, user1.Client, "it-quota-1", templateId, user1.Id)
	harness.DeleteSpaceAsync(t, admin.Client, first)

	if _, code, err := user1.Client.CreateSpace(ctx, &apiclient.SpaceRequest{
		Name: "it-quota-2", TemplateId: templateId, UserId: user1.Id, Shell: "bash",
	}); err == nil {
		t.Fatal("space beyond quota was created")
	} else if code != 403 && code != 400 {
		t.Fatalf("quota status = %d, want 403/400", code)
	}
}
