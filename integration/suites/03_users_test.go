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

	// user2 has no user-management permission.
	_, code, err := user2.Client.CreateUser(ctx, &apiclient.CreateUserRequest{
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
		t.Fatal("user2 created a user without permission")
	}
	mustEqual(t, "forbidden status", code, 403)

	// Reading templates is allowed for all users (the space form needs the
	// picker); writing them is not.
	if _, _, err := user2.Client.GetTemplates(ctx); err != nil {
		t.Fatalf("user2 should be able to list templates: %v", err)
	}
	if _, code, err := user2.Client.CreateTemplate(ctx, &apiclient.TemplateCreateRequest{
		Name: uniqueName("it-nope"), Platform: "container", Active: true,
		Job: "image: paularlott/knot-ubuntu:26.04", MaxUptimeUnit: "disabled",
	}); err == nil {
		t.Fatal("user2 created a template without permission")
	} else if code != 403 {
		t.Fatalf("template create status = %d, want 403", code)
	}
}

func TestUserQuotaMaxSpaces(t *testing.T) {
	harness.Feature(t, "quotas")
	ctx, cancel := testCtx(60)
	defer cancel()

	// user2 default quota is 20; create a dedicated user with quota 1.
	quotaUser, err := harness.CreateUser(server, admin, uniqueName("it-quota"), harness.TesterPermissions())
	if err != nil {
		t.Fatalf("create quota user: %v", err)
	}
	defer func() {
		ctx, cancel := testCtx(60)
		_ = admin.Client.DeleteUser(ctx, quotaUser.Id)
		cancel()
	}()

	// Shrink the quota to 1 (already has zero spaces).
	quotaUserInfo, err := admin.Client.GetUser(ctx, quotaUser.Id)
	if err != nil {
		t.Fatalf("get quota user: %v", err)
	}
	upd := &apiclient.UpdateUserRequest{
		Username:       quotaUser.Username,
		Email:          quotaUser.Email,
		Roles:          quotaUserInfo.Roles,
		Groups:         quotaUserInfo.Groups,
		Active:         true,
		MaxSpaces:      1,
		ComputeUnits:   5,
		StorageUnits:   5,
		MaxTunnels:     2,
		PreferredShell: "bash",
		Timezone:       "UTC",
	}
	if err := admin.Client.UpdateUser(ctx, quotaUser.Id, upd); err != nil {
		t.Fatalf("update quota user: %v", err)
	}

	first := harness.CreateSpace(t, quotaUser.Client, "it-quota-1", templateId, quotaUser.Id)
	harness.DeleteSpaceAsync(t, admin.Client, first)

	if _, code, err := quotaUser.Client.CreateSpace(ctx, &apiclient.SpaceRequest{
		Name: "it-quota-2", TemplateId: templateId, UserId: quotaUser.Id, Shell: "bash",
	}); err == nil {
		t.Fatal("space beyond quota was created")
	} else if code != 403 && code != 400 {
		t.Fatalf("quota status = %d, want 403/400", code)
	}
}
