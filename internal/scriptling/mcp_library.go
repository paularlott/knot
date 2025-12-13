package scriptling

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/paularlott/scriptling/object"
)

// GetMCPLibrary returns the MCP helper library for scriptling
func GetMCPLibrary(mcpParams map[string]string) *object.Library {
	functions := map[string]*object.Builtin{
		"get": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpGet(mcpParams, args...)
			},
			HelpText: "get(name[, default]) - Get MCP parameter value with automatic type conversion",
		},
		"return_string": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpReturnString(args...)
			},
			HelpText: "return_string(value) - Return a string result and exit",
		},
		"return_object": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpReturnObject(args...)
			},
			HelpText: "return_object(value) - Return a structured object as JSON and exit",
		},
		"return_error": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpReturnError(args...)
			},
			HelpText: "return_error(message) - Return an error message and exit with error code",
		},
	}
	
	return object.NewLibrary(functions, nil, "MCP helper functions for tool scripts")
}

// mcpGet retrieves a parameter value with automatic type conversion
func mcpGet(mcpParams map[string]string, args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Null{}
	}
	
	name := args[0].(*object.String).Value
	value, ok := mcpParams[name]
	if !ok {
		value = ""
	}
	
	// If not found, return default or NULL
	if value == "" {
		if len(args) == 2 {
			return args[1]
		}
		return &object.Null{}
	}
	
	// Try to parse as JSON first (for arrays/objects)
	var jsonValue interface{}
	if err := json.Unmarshal([]byte(value), &jsonValue); err == nil {
		return interfaceToObject(jsonValue)
	}
	
	// Try as number
	if num, err := strconv.ParseFloat(value, 64); err == nil {
		if num == float64(int64(num)) {
			return &object.Integer{Value: int64(num)}
		}
		return &object.Float{Value: num}
	}
	
	// Try as boolean
	if value == "true" {
		return &object.Boolean{Value: true}
	}
	if value == "false" {
		return &object.Boolean{Value: false}
	}
	
	// Return as string
	return &object.String{Value: value}
}

// mcpReturnString returns a string value (script should exit after this)
func mcpReturnString(args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.String{Value: ""}
	}
	return args[0]
}

// mcpReturnObject returns an object as JSON (script should exit after this)
func mcpReturnObject(args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Null{}
	}
	return args[0]
}

// mcpReturnError returns an error (script should exit after this)
func mcpReturnError(args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.String{Value: "Error"}
	}
	return args[0]
}

// interfaceToObject converts a Go interface{} to a scriptling object
func interfaceToObject(v interface{}) object.Object {
	switch val := v.(type) {
	case nil:
		return &object.Null{}
	case bool:
		return &object.Boolean{Value: val}
	case float64:
		if val == float64(int64(val)) {
			return object.NewInteger(int64(val))
		}
		return &object.Float{Value: val}
	case string:
		return &object.String{Value: val}
	case []interface{}:
		elements := make([]object.Object, len(val))
		for i, elem := range val {
			elements[i] = interfaceToObject(elem)
		}
		return &object.List{Elements: elements}
	case map[string]interface{}:
		pairs := make(map[string]object.DictPair)
		for k, v := range val {
			key := &object.String{Value: k}
			value := interfaceToObject(v)
			pairs[k] = object.DictPair{Key: key, Value: value}
		}
		return &object.Dict{Pairs: pairs}
	default:
		return &object.Null{}
	}
}
