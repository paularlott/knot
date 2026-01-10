package scriptling

import (
	"fmt"

	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/object"
)

// GetIntAsUint16 safely extracts an integer argument at the given index and converts to uint16.
// Returns the uint16 value and nil error on success, or 0 and error object on failure.
func GetIntAsUint16(args []object.Object, index int, name string) (uint16, object.Object) {
	i, err := scriptling.GetInt(args, index, name)
	if err != nil {
		return 0, err
	}
	if i < 0 || i > 65535 {
		return 0, &object.Error{Message: fmt.Sprintf("%s: must be between 0 and 65535", name)}
	}
	return uint16(i), nil
}
