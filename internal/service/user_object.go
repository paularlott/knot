package service

import (
	"strings"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/scriptling/object"
)

// userClass is the class of the User instance knot.identity.user() returns
// (and the `user` global that user-created MCP tool scripts still bind).
// Built once; instances carry the per-dispatch identity as fields, and the
// methods answer permission questions with the same semantics knot's own
// checks use (admins pass everything). Plugin handlers do not get a `user`
// global — they receive identity as request["user"] data — so this class
// is the authoritative surface, reached through knot.identity.
var userClass = buildUserClass()

func buildUserClass() *object.Class {
	builder := object.NewClassBuilder("User")

	stringListField := func(self *object.Instance, field string) []string {
		out := []string{}
		if list, ok := self.Field(field).(*object.List); ok {
			for _, item := range list.Elements {
				if s, ok := item.(*object.String); ok {
					out = append(out, s.StringValue())
				}
			}
		}
		return out
	}
	boolField := func(self *object.Instance, field string) bool {
		b, _ := self.Field(field).(*object.Boolean)
		return b != nil && b.BoolValue()
	}

	// One method, three argument shapes: the argument's type picks the
	// list. An integer is a built-in permission id (the knot.permission
	// constants); a string is either a qualified plugin grant ("plugin."
	// prefix) or a built-in's stable snake_case key — the same strings
	// user.permissions holds. Admins pass every check.
	builder.Method("has_permission", func(self *object.Instance, name object.Object) bool {
		if boolField(self, "is_admin") {
			return true
		}
		holds := func(field, want string) bool {
			for _, p := range stringListField(self, field) {
				if p == want {
					return true
				}
			}
			return false
		}
		switch v := name.(type) {
		case *object.Integer:
			key := model.PermissionKey(uint16(v.IntValue()))
			return key != "" && holds("permissions", key)
		case *object.String:
			if strings.HasPrefix(v.StringValue(), "plugin.") {
				return holds("plugin_permissions", v.StringValue())
			}
			return holds("permissions", v.StringValue())
		}
		return false
	})
	builder.Method("in_group", func(self *object.Instance, name string) bool {
		for _, g := range stringListField(self, "groups") {
			if g == name {
				return true
			}
		}
		return false
	})

	return builder.Build()
}

// NewUserObject builds the requesting user as a User instance — fields id,
// name, is_admin, groups, permissions (stable snake_case keys like
// "manage_spaces") and plugin_permissions (qualified grants) — with
// has_permission and in_group methods. It backs knot.identity.user() (the
// authoritative identity surface in plugin and tool code) and the `user`
// global user-created MCP tool scripts bind. has_permission's argument
// picks the check: an integer is a built-in permission id (the
// knot.permission constants), a "plugin."-prefixed string a qualified
// grant, any other string a built-in key. It complements — never replaces —
// the metadata gates knot enforces before a handler runs.
func NewUserObject(user *model.User) object.Object {
	if user == nil {
		return object.NewInstanceWithFields(userClass, map[string]object.Object{})
	}
	permissions := make([]object.Object, 0)
	for _, p := range user.GrantedPermissionKeys() {
		permissions = append(permissions, object.NewString(p))
	}
	pluginPermissions := make([]object.Object, 0)
	for _, p := range user.GrantedPluginPermissions() {
		pluginPermissions = append(pluginPermissions, object.NewString(p))
	}
	groups := make([]object.Object, 0)
	for _, g := range user.Groups {
		groups = append(groups, object.NewString(g))
	}
	return object.NewInstanceWithFields(userClass, map[string]object.Object{
		"id":                 object.NewString(user.Id),
		"name":               object.NewString(user.Username),
		"is_admin":           object.NewBoolean(user.IsAdmin()),
		"groups":             &object.List{Elements: groups},
		"permissions":        &object.List{Elements: permissions},
		"plugin_permissions": &object.List{Elements: pluginPermissions},
	})
}
