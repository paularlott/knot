package scriptling

import (
	"context"

	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// UserSecretResolver reads a personal secret through the agent running in
// the space. The agentlink package installs it; scripts outside a space
// never get this library and use the embedded Python knot.secret, which
// refuses.
var UserSecretResolver func(path, field string) (string, error)

// GetSecretLibrary returns the knot.secret library for scripts running in a
// space. It shadows the embedded Python module: personal secrets are only
// read over the agent link, never through the REST API.
func GetSecretLibrary() *object.Library {
	builder := object.NewLibraryBuilder("knot.secret", "Read your own personal secrets (Knot Pro)")

	builder.FunctionWithHelp("get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		if err := errors.MinArgs(args, 1); err != nil {
			return err
		}
		path, err := args[0].AsString()
		if err != nil {
			return errors.ParameterError("path", err)
		}

		field := ""
		if len(args) > 1 {
			if field, err = args[1].AsString(); err != nil {
				return errors.ParameterError("field", err)
			}
		} else if f := kwargs.Get("field"); f != nil {
			if field, err = f.AsString(); err != nil {
				return errors.ParameterError("field", err)
			}
		}

		if UserSecretResolver == nil {
			return &object.Error{Message: "personal secrets can only be read inside a knot space"}
		}
		value, rerr := UserSecretResolver(path, field)
		if rerr != nil {
			return &object.Error{Message: rerr.Error()}
		}
		return object.NewString(value)
	}, `get(path) - Read one of your own secrets by its 1Password reference, op://vault/item/field (op://vault/item/section/field for a field in a section)`)

	return builder.Build()
}
