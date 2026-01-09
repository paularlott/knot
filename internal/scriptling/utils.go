package scriptling

import (
	"fmt"

	"github.com/paularlott/scriptling/object"
)

// GetString safely extracts a string argument at the given index.
// Returns the string value and nil error on success, or empty string and error object on failure.
func GetString(args []object.Object, index int, name string) (string, object.Object) {
	if index >= len(args) {
		return "", &object.Error{Message: fmt.Sprintf("%s: missing argument", name)}
	}
	if s, ok := args[index].AsString(); ok {
		return s, nil
	}
	return "", &object.Error{Message: fmt.Sprintf("%s: must be a string", name)}
}

// GetInt safely extracts an integer argument at the given index.
// Returns the int64 value and nil error on success, or 0 and error object on failure.
func GetInt(args []object.Object, index int, name string) (int64, object.Object) {
	if index >= len(args) {
		return 0, &object.Error{Message: fmt.Sprintf("%s: missing argument", name)}
	}
	if i, ok := args[index].AsInt(); ok {
		return i, nil
	}
	return 0, &object.Error{Message: fmt.Sprintf("%s: must be an integer", name)}
}

// GetIntAsUint16 safely extracts an integer argument at the given index and converts to uint16.
// Returns the uint16 value and nil error on success, or 0 and error object on failure.
func GetIntAsUint16(args []object.Object, index int, name string) (uint16, object.Object) {
	i, err := GetInt(args, index, name)
	if err != nil {
		return 0, err
	}
	if i < 0 || i > 65535 {
		return 0, &object.Error{Message: fmt.Sprintf("%s: must be between 0 and 65535", name)}
	}
	return uint16(i), nil
}

// GetBool safely extracts a boolean argument at the given index.
// Returns the bool value and nil error on success, or false and error object on failure.
func GetBool(args []object.Object, index int, name string) (bool, object.Object) {
	if index >= len(args) {
		return false, &object.Error{Message: fmt.Sprintf("%s: missing argument", name)}
	}
	if b, ok := args[index].AsBool(); ok {
		return b, nil
	}
	return false, &object.Error{Message: fmt.Sprintf("%s: must be a boolean", name)}
}

// GetList safely extracts a list argument at the given index.
// Returns the list value and nil error on success, or nil and error object on failure.
func GetList(args []object.Object, index int, name string) ([]object.Object, object.Object) {
	if index >= len(args) {
		return nil, &object.Error{Message: fmt.Sprintf("%s: missing argument", name)}
	}
	if l, ok := args[index].AsList(); ok {
		return l, nil
	}
	return nil, &object.Error{Message: fmt.Sprintf("%s: must be a list", name)}
}

// GetDict safely extracts a dict argument at the given index.
// Returns the dict value and nil error on success, or nil and error object on failure.
func GetDict(args []object.Object, index int, name string) (map[string]object.Object, object.Object) {
	if index >= len(args) {
		return nil, &object.Error{Message: fmt.Sprintf("%s: missing argument", name)}
	}
	if d, ok := args[index].AsDict(); ok {
		return d, nil
	}
	return nil, &object.Error{Message: fmt.Sprintf("%s: must be a dict", name)}
}

// GetStringFromKwargs safely extracts a string from kwargs.
// Returns the string value and true if found, or empty string and false if not present.
// Returns error object if the key exists but is not a string.
func GetStringFromKwargs(kwargs map[string]object.Object, key string) (string, bool, object.Object) {
	obj, found := kwargs[key]
	if !found {
		return "", false, nil
	}
	if s, ok := obj.AsString(); ok {
		return s, true, nil
	}
	return "", false, &object.Error{Message: fmt.Sprintf("%s: must be a string", key)}
}

// GetIntFromKwargs safely extracts an integer from kwargs.
// Returns the int64 value and true if found, or 0 and false if not present.
// Returns error object if the key exists but is not an integer.
func GetIntFromKwargs(kwargs map[string]object.Object, key string) (int64, bool, object.Object) {
	obj, found := kwargs[key]
	if !found {
		return 0, false, nil
	}
	if i, ok := obj.AsInt(); ok {
		return i, true, nil
	}
	return 0, false, &object.Error{Message: fmt.Sprintf("%s: must be an integer", key)}
}

// GetBoolFromKwargs safely extracts a boolean from kwargs.
// Returns the bool value and true if found, or false and false if not present.
// Returns error object if the key exists but is not a boolean.
func GetBoolFromKwargs(kwargs map[string]object.Object, key string) (bool, bool, object.Object) {
	obj, found := kwargs[key]
	if !found {
		return false, false, nil
	}
	if b, ok := obj.AsBool(); ok {
		return b, true, nil
	}
	return false, false, &object.Error{Message: fmt.Sprintf("%s: must be a boolean", key)}
}

// GetListFromKwargs safely extracts a list from kwargs.
// Returns the list value and true if found, or nil and false if not present.
// Returns error object if the key exists but is not a list.
func GetListFromKwargs(kwargs map[string]object.Object, key string) ([]object.Object, bool, object.Object) {
	obj, found := kwargs[key]
	if !found {
		return nil, false, nil
	}
	if l, ok := obj.AsList(); ok {
		return l, true, nil
	}
	return nil, false, &object.Error{Message: fmt.Sprintf("%s: must be a list", key)}
}

// RequireMinArgs checks that there are at least minArgs arguments.
// Returns an error object if not enough arguments, nil if OK.
func RequireMinArgs(args []object.Object, minArgs int, funcName string) object.Object {
	if len(args) < minArgs {
		return &object.Error{Message: fmt.Sprintf("%s() requires at least %d argument(s)", funcName, minArgs)}
	}
	return nil
}
