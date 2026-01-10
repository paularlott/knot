package scriptling

import (
	"context"
	"fmt"

	"github.com/paularlott/mcp/toon"
	scriptlib "github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/object"
)

// GetToonBuiltins returns the shared toon encode/decode functions for MCP libraries
func GetToonBuiltins() map[string]*object.Builtin {
	return map[string]*object.Builtin{
		"toon_encode": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				if len(args) < 1 {
					return &object.String{Value: "Error: toon_encode requires 1 argument"}
				}
				goValue := scriptlib.ToGo(args[0])
				encoded, err := toon.Encode(goValue)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("Error encoding to toon: %v", err)}
				}
				return &object.String{Value: encoded}
			},
			HelpText: "toon_encode(value) - Encode a value to toon format string.",
		},
		"toon_decode": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				if len(args) < 1 {
					return &object.String{Value: "Error: toon_decode requires 1 argument"}
				}
				str, ok := args[0].(*object.String)
				if !ok {
					return &object.String{Value: "Error: toon_decode argument must be a string"}
				}
				decoded, err := toon.Decode(str.Value)
				if err != nil {
					return &object.String{Value: fmt.Sprintf("Error decoding from toon: %v", err)}
				}
				return scriptlib.FromGo(decoded)
			},
			HelpText: "toon_decode(string) - Decode a toon format string to a value.",
		},
	}
}
