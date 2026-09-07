package service

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/scriptling/object"
)

// TestNewUserObjectFields pins the object's field contents straight from
// the model: stable keys and qualified grants flow through the Granted*
// derivation, admins hold every built-in key, and a nil user (no identity
// in context) yields an all-empty instance every check refuses.
func TestNewUserObjectFields(t *testing.T) {
	model.SetRoleCache([]*model.Role{{
		Id:                "role-x",
		Name:              "X",
		Permissions:       []uint16{model.PermissionUseMCPServer},
		PluginPermissions: []string{"plugin.metrics.read"},
	}})

	inst, ok := NewUserObject(&model.User{
		Username: "kai", Id: "u-1", Groups: []string{"platform"}, Roles: []string{"role-x"},
	}).(*object.Instance)
	if !ok {
		t.Fatalf("NewUserObject did not return an instance: %T", NewUserObject(nil))
	}
	if got := fieldString(inst, "id"); got != "u-1" {
		t.Errorf("id = %q, want u-1", got)
	}
	if got := fieldString(inst, "name"); got != "kai" {
		t.Errorf("name = %q, want kai", got)
	}
	if got := fieldBool(inst, "is_admin"); got {
		t.Error("is_admin = true for a plain role user")
	}
	if groups := fieldList(inst, "groups"); len(groups) != 1 || groups[0] != "platform" {
		t.Errorf("groups = %v, want [platform]", groups)
	}
	perms := fieldList(inst, "permissions")
	if !containsString(perms, "use_mcp_server") || containsString(perms, "manage_spaces") {
		t.Errorf("permissions = %v, want use_mcp_server held and manage_spaces absent", perms)
	}
	if grants := fieldList(inst, "plugin_permissions"); len(grants) != 1 || grants[0] != "plugin.metrics.read" {
		t.Errorf("plugin_permissions = %v, want [plugin.metrics.read]", grants)
	}

	admin, ok := NewUserObject(&model.User{
		Username: "root", Id: "u-2", Roles: []string{model.RoleAdminUUID},
	}).(*object.Instance)
	if !ok {
		t.Fatal("admin object is not an instance")
	}
	if !fieldBool(admin, "is_admin") {
		t.Error("is_admin = false for the admin role")
	}
	if perms := fieldList(admin, "permissions"); !containsString(perms, "manage_spaces") {
		t.Errorf("admin permissions = %v, want every built-in key including manage_spaces", perms)
	}

	nilInst, ok := NewUserObject(nil).(*object.Instance)
	if !ok {
		t.Fatal("nil-user object is not an instance")
	}
	if got := fieldString(nilInst, "name"); got != "" {
		t.Errorf("nil user name = %q, want empty", got)
	}
	if perms := fieldList(nilInst, "permissions"); len(perms) != 0 {
		t.Errorf("nil user permissions = %v, want empty", perms)
	}
}

func fieldString(inst *object.Instance, name string) string {
	if s, ok := inst.Field(name).(*object.String); ok {
		return s.StringValue()
	}
	return ""
}

func fieldBool(inst *object.Instance, name string) bool {
	b, ok := inst.Field(name).(*object.Boolean)
	return ok && b.BoolValue()
}

func fieldList(inst *object.Instance, name string) []string {
	out := []string{}
	if list, ok := inst.Field(name).(*object.List); ok {
		for _, item := range list.Elements {
			if s, ok := item.(*object.String); ok {
				out = append(out, s.StringValue())
			}
		}
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
