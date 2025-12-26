package scriptling

import (
	"fmt"

	"github.com/paularlott/scriptling/object"
)

// ChatMessage represents a chat message
type ChatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Messages []ChatMessage `json:"messages"`
}

// ChatCompletionResponse represents a chat completion response
type ChatCompletionResponse struct {
	Content string `json:"content"`
}

// Tool represents a tool and its parameters
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCallRequest represents a tool call request
type ToolCallRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolCallResponse represents a tool call response
type ToolCallResponse struct {
	Content interface{} `json:"content"`
}

// Helper functions to convert between scriptling objects and Go types

func convertToScriptlingObject(v interface{}) object.Object {
	switch val := v.(type) {
	case string:
		return &object.String{Value: val}
	case int:
		return &object.Integer{Value: int64(val)}
	case int64:
		return &object.Integer{Value: val}
	case float64:
		return &object.Float{Value: val}
	case bool:
		return &object.Boolean{Value: val}
	case map[string]interface{}:
		pairs := make(map[string]object.DictPair, len(val))
		for k, v := range val {
			pairs[k] = object.DictPair{
				Key:   &object.String{Value: k},
				Value: convertToScriptlingObject(v),
			}
		}
		return &object.Dict{Pairs: pairs}
	case []interface{}:
		elements := make([]object.Object, 0, len(val))
		for _, v := range val {
			elements = append(elements, convertToScriptlingObject(v))
		}
		return &object.List{Elements: elements}
	case nil:
		return &object.Null{}
	default:
		// For other types, convert to string representation
		return &object.String{Value: fmt.Sprintf("%v", val)}
	}
}

func convertFromScriptlingObject(obj object.Object) interface{} {
	switch o := obj.(type) {
	case *object.String:
		return o.Value
	case *object.Integer:
		return o.Value
	case *object.Float:
		return o.Value
	case *object.Boolean:
		return o.Value
	case *object.Null:
		return nil
	case *object.Dict:
		result := make(map[string]interface{})
		for _, pair := range o.Pairs {
			key := pair.Key.(*object.String).Value
			result[key] = convertFromScriptlingObject(pair.Value)
		}
		return result
	case *object.List:
		result := make([]interface{}, 0, len(o.Elements))
		for _, elem := range o.Elements {
			result = append(result, convertFromScriptlingObject(elem))
		}
		return result
	default:
		return nil
	}
}
