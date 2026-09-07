package scriptling

import (
	"context"

	"github.com/paularlott/scriptling/object"
)

// GetIdentityLibrary builds knot.identity: the requesting user for code
// that runs inside imported modules (plugin peers, lib scripts), where the
// dispatch globals — bound on the main program's scope — are not visible.
// user() returns the same User instance the `user` global holds, so module
// code can self-gate exactly like handler code does.
//
// The instance is captured at registration: environments that rebind
// identity (pooled plugin envs per lease, server envs per execution)
// re-register the library with the current user.
func GetIdentityLibrary(user object.Object) *object.Library {
	builder := object.NewLibraryBuilder("knot.identity", "The requesting user for module code")

	builder.FunctionWithHelp("user", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return user
	}, "user() - the User instance the `user` global holds (id, name, is_admin, groups, permissions, plugin_permissions; has_permission, in_group)")

	return builder.Build()
}
