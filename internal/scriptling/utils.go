package scriptling

import (
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// GetIntAsUint16 safely extracts an integer argument at the given index and converts to uint16.
// Returns the uint16 value and nil error on success, or 0 and error object on failure.
func GetIntAsUint16(args []object.Object, index int, name string) (uint16, object.Object) {
	if index >= len(args) {
		return 0, errors.NewError("%s: argument index %d out of range", name, index)
	}

	i, err := args[index].AsInt()
	if err != nil {
		return 0, errors.ParameterError(name, err)
	}

	if i < 0 || i > 65535 {
		return 0, errors.NewError("%s: must be between 0 and 65535", name)
	}

	return uint16(i), nil
}
