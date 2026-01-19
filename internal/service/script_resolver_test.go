package service

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

// TestCanUserExecuteScript_UserScript_Owner tests that a user can execute their own script
func TestCanUserExecuteScript_UserScript_Owner(t *testing.T) {
	user := &model.User{
		Id:       "user1",
		Username: "user1",
		Roles:    []string{model.RoleAdminUUID}, // Admin role has all permissions
	}

	script := &model.Script{
		Id:       "script1",
		Name:     "user_script",
		UserId:   "user1",
		Active:   true,
		IsDeleted: false,
	}

	// Set up the role cache with ExecuteOwnScripts permission
	model.SetRoleCache([]*model.Role{
		{
			Id:    model.RoleAdminUUID,
			Name:  "Admin",
			Permissions: []uint16{
				model.PermissionExecuteOwnScripts,
			},
		},
	})

	result := CanUserExecuteScript(user, script)
	if !result {
		t.Error("User should be able to execute their own script with ExecuteOwnScripts permission")
	}
}

// TestCanUserExecuteScript_UserScript_NotOwner tests that a user cannot execute another user's script
func TestCanUserExecuteScript_UserScript_NotOwner(t *testing.T) {
	user1 := &model.User{
		Id:       "user1",
		Username: "user1",
		Roles:    []string{model.RoleAdminUUID},
	}

	user2 := &model.User{
		Id:       "user2",
		Username: "user2",
		Roles:    []string{model.RoleAdminUUID},
	}

	script := &model.Script{
		Id:       "script1",
		Name:     "user1_script",
		UserId:   "user1",
		Active:   true,
		IsDeleted: false,
	}

	// Set up the role cache with ExecuteOwnScripts permission
	model.SetRoleCache([]*model.Role{
		{
			Id:    model.RoleAdminUUID,
			Name:  "Admin",
			Permissions: []uint16{
				model.PermissionExecuteOwnScripts,
			},
		},
	})

	// user2 should NOT be able to execute user1's script
	result := CanUserExecuteScript(user2, script)
	if result {
		t.Error("User should NOT be able to execute another user's script")
	}

	// user1 should be able to execute their own script
	result = CanUserExecuteScript(user1, script)
	if !result {
		t.Error("User should be able to execute their own script")
	}
}

// TestCanUserExecuteScript_GlobalScript_WithPermission tests that a user with ExecuteScripts can execute global scripts
func TestCanUserExecuteScript_GlobalScript_WithPermission(t *testing.T) {
	user := &model.User{
		Id:       "user1",
		Username: "user1",
		Roles:    []string{model.RoleAdminUUID},
	}

	script := &model.Script{
		Id:       "script1",
		Name:     "global_script",
		UserId:   "", // Global script
		Active:   true,
		IsDeleted: false,
		Groups:   []string{},
	}

	// Set up the role cache with ExecuteScripts permission
	model.SetRoleCache([]*model.Role{
		{
			Id:    model.RoleAdminUUID,
			Name:  "Admin",
			Permissions: []uint16{
				model.PermissionExecuteScripts,
			},
		},
	})

	result := CanUserExecuteScript(user, script)
	if !result {
		t.Error("User with ExecuteScripts permission should be able to execute global script")
	}
}

// TestCanUserExecuteScript_GlobalScript_WithoutPermission tests that a user without ExecuteScripts cannot execute global scripts
func TestCanUserExecuteScript_GlobalScript_WithoutPermission(t *testing.T) {
	user := &model.User{
		Id:       "user1",
		Username: "user1",
		Roles:    []string{"role-no-permissions"},
	}

	script := &model.Script{
		Id:       "script1",
		Name:     "global_script",
		UserId:   "", // Global script
		Active:   true,
		IsDeleted: false,
		Groups:   []string{},
	}

	// Set up the role cache without ExecuteScripts permission
	model.SetRoleCache([]*model.Role{
		{
			Id:          "role-no-permissions",
			Name:        "No Permissions",
			Permissions: []uint16{},
		},
	})

	result := CanUserExecuteScript(user, script)
	if result {
		t.Error("User without ExecuteScripts permission should NOT be able to execute global script")
	}
}

// TestCanUserExecuteScript_GlobalScript_WithGroupRestriction tests group-based access control
func TestCanUserExecuteScript_GlobalScript_WithGroupRestriction(t *testing.T) {
	userInGroup := &model.User{
		Id:       "user1",
		Username: "user1",
		Groups:   []string{"developers"},
		Roles:    []string{"role-execute-only"},
	}

	userNotInGroup := &model.User{
		Id:       "user2",
		Username: "user2",
		Groups:   []string{"other-group"},
		Roles:    []string{"role-execute-only"},
	}

	script := &model.Script{
		Id:       "script1",
		Name:     "global_script",
		UserId:   "", // Global script
		Active:   true,
		IsDeleted: false,
		Groups:   []string{"developers"},
	}

	// Set up the role cache with ExecuteScripts but NOT ManageScripts
	model.SetRoleCache([]*model.Role{
		{
			Id:          "role-execute-only",
			Name:        "Execute Only",
			Permissions: []uint16{model.PermissionExecuteScripts},
		},
	})

	// User in group should be able to execute
	result := CanUserExecuteScript(userInGroup, script)
	if !result {
		t.Error("User in the required group should be able to execute the script")
	}

	// User NOT in group should NOT be able to execute
	result = CanUserExecuteScript(userNotInGroup, script)
	if result {
		t.Error("User NOT in the required group should NOT be able to execute the script")
	}
}

// TestCanUserExecuteScript_GlobalScript_AdminBypassesGroups tests that admins bypass group checks
func TestCanUserExecuteScript_GlobalScript_AdminBypassesGroups(t *testing.T) {
	user := &model.User{
		Id:       "user1",
		Username: "user1",
		Groups:   []string{"other-group"}, // NOT in the script's group
		Roles:    []string{"role-admin"},
	}

	script := &model.Script{
		Id:       "script1",
		Name:     "global_script",
		UserId:   "", // Global script
		Active:   true,
		IsDeleted: false,
		Groups:   []string{"developers"},
	}

	// Set up the role cache with both ExecuteScripts AND ManageScripts
	model.SetRoleCache([]*model.Role{
		{
			Id:    "role-admin",
			Name:  "Admin",
			Permissions: []uint16{
				model.PermissionExecuteScripts,
				model.PermissionManageScripts,
			},
		},
	})

	result := CanUserExecuteScript(user, script)
	if !result {
		t.Error("Admin with ManageScripts permission should bypass group restrictions")
	}
}

// TestCanUserExecuteScript_UserScript_NoPermission tests that a user cannot execute their own script without ExecuteOwnScripts
func TestCanUserExecuteScript_UserScript_NoPermission(t *testing.T) {
	user := &model.User{
		Id:       "user1",
		Username: "user1",
		Roles:    []string{"role-no-permissions"},
	}

	script := &model.Script{
		Id:       "script1",
		Name:     "user_script",
		UserId:   "user1",
		Active:   true,
		IsDeleted: false,
	}

	// Set up the role cache without ExecuteOwnScripts permission
	model.SetRoleCache([]*model.Role{
		{
			Id:          "role-no-permissions",
			Name:        "No Permissions",
			Permissions: []uint16{},
		},
	})

	result := CanUserExecuteScript(user, script)
	if result {
		t.Error("User without ExecuteOwnScripts permission should NOT be able to execute their own script")
	}
}

// TestIsValidForZone_NoZones tests that a script with no zones is valid for all zones
func TestIsValidForZone_NoZones(t *testing.T) {
	script := &model.Script{
		Id:        "script1",
		Name:      "global_script",
		Zones:     []string{},
		Active:    true,
		IsDeleted: false,
	}

	result := script.IsValidForZone("zone1")
	if !result {
		t.Error("Script with no zones should be valid for any zone")
	}

	result = script.IsValidForZone("")
	if !result {
		t.Error("Script with no zones should be valid for empty zone")
	}
}

// TestIsValidForZone_ExplicitZone tests scripts with explicit zone restrictions
func TestIsValidForZone_ExplicitZone(t *testing.T) {
	script := &model.Script{
		Id:        "script1",
		Name:      "zone1_script",
		Zones:     []string{"zone1", "zone2"},
		Active:    true,
		IsDeleted: false,
	}

	// Should be valid for zone1
	result := script.IsValidForZone("zone1")
	if !result {
		t.Error("Script should be valid for zone1")
	}

	// Should be valid for zone2
	result = script.IsValidForZone("zone2")
	if !result {
		t.Error("Script should be valid for zone2")
	}

	// Should NOT be valid for zone3
	result = script.IsValidForZone("zone3")
	if result {
		t.Error("Script should NOT be valid for zone3")
	}
}

// TestIsValidForZone_NegatedZone tests scripts with negated zone restrictions
func TestIsValidForZone_NegatedZone(t *testing.T) {
	script := &model.Script{
		Id:        "script1",
		Name:      "not_zone1_script",
		Zones:     []string{"!zone1"},
		Active:    true,
		IsDeleted: false,
	}

	// Should NOT be valid for zone1
	result := script.IsValidForZone("zone1")
	if result {
		t.Error("Script with !zone1 should NOT be valid for zone1")
	}

	// Should NOT be valid for zone2 (not explicitly allowed)
	result = script.IsValidForZone("zone2")
	if result {
		t.Error("Script with only negated zones should NOT be valid for other zones")
	}
}

// TestIsValidForZone_MixedZones tests scripts with both positive and negated zones
func TestIsValidForZone_MixedZones(t *testing.T) {
	script := &model.Script{
		Id:        "script1",
		Name:      "mixed_zones_script",
		Zones:     []string{"zone1", "zone2", "!zone2"},
		Active:    true,
		IsDeleted: false,
	}

	// Should be valid for zone1
	result := script.IsValidForZone("zone1")
	if !result {
		t.Error("Script should be valid for zone1")
	}

	// Should NOT be valid for zone2 (negation takes precedence)
	result = script.IsValidForZone("zone2")
	if result {
		t.Error("Script should NOT be valid for zone2 (negated)")
	}

	// Should NOT be valid for zone3 (not in allowed list)
	result = script.IsValidForZone("zone3")
	if result {
		t.Error("Script should NOT be valid for zone3")
	}
}

// TestIsGlobalScript tests identifying global scripts
func TestIsGlobalScript(t *testing.T) {
	globalScript := &model.Script{
		Id:     "script1",
		Name:   "global_script",
		UserId: "", // Empty UserId means global
	}

	userScript := &model.Script{
		Id:     "script2",
		Name:   "user_script",
		UserId: "user1",
	}

	if !globalScript.IsGlobalScript() {
		t.Error("Script with empty UserId should be a global script")
	}

	if userScript.IsGlobalScript() {
		t.Error("Script with non-empty UserId should NOT be a global script")
	}
}

// TestIsUserScript tests identifying user scripts
func TestIsUserScript(t *testing.T) {
	globalScript := &model.Script{
		Id:     "script1",
		Name:   "global_script",
		UserId: "", // Empty UserId means global
	}

	userScript := &model.Script{
		Id:     "script2",
		Name:   "user_script",
		UserId: "user1",
	}

	if globalScript.IsUserScript() {
		t.Error("Script with empty UserId should NOT be a user script")
	}

	if !userScript.IsUserScript() {
		t.Error("Script with non-empty UserId should be a user script")
	}
}

// TestPermissionModelExecuteOwnVsExecuteScripts tests the distinction between ExecuteOwnScripts and ExecuteScripts
func TestPermissionModelExecuteOwnVsExecuteScripts(t *testing.T) {
	userWithOwnScripts := &model.User{
		Id:       "user1",
		Username: "user1",
		Roles:    []string{"role-own-scripts"},
	}

	userWithGlobalScripts := &model.User{
		Id:       "user2",
		Username: "user2",
		Roles:    []string{"role-global-scripts"},
	}

	globalScript := &model.Script{
		Id:       "script1",
		Name:     "global_script",
		UserId:   "", // Global
		Active:   true,
		IsDeleted: false,
		Groups:   []string{},
	}

	userScript := &model.Script{
		Id:       "script2",
		Name:     "user_script",
		UserId:   "user1",
		Active:   true,
		IsDeleted: false,
	}

	// Set up roles
	model.SetRoleCache([]*model.Role{
		{
			Id:          "role-own-scripts",
			Name:        "Own Scripts Only",
			Permissions: []uint16{model.PermissionExecuteOwnScripts},
		},
		{
			Id:          "role-global-scripts",
			Name:        "Global Scripts Only",
			Permissions: []uint16{model.PermissionExecuteScripts},
		},
	})

	// User with ExecuteOwnScripts can execute their own script
	result := CanUserExecuteScript(userWithOwnScripts, userScript)
	if !result {
		t.Error("User with ExecuteOwnScripts should be able to execute their own script")
	}

	// User with ExecuteOwnScripts CANNOT execute global scripts
	result = CanUserExecuteScript(userWithOwnScripts, globalScript)
	if result {
		t.Error("User with only ExecuteOwnScripts should NOT be able to execute global scripts")
	}

	// User with ExecuteScripts can execute global scripts
	result = CanUserExecuteScript(userWithGlobalScripts, globalScript)
	if !result {
		t.Error("User with ExecuteScripts should be able to execute global scripts")
	}

	// User with ExecuteScripts CANNOT execute user scripts (not their own)
	result = CanUserExecuteScript(userWithGlobalScripts, userScript)
	if result {
		t.Error("User with only ExecuteScripts should NOT be able to execute other users' scripts")
	}
}
