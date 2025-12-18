package scriptling

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/paularlott/knot/internal/openai"
	"github.com/paularlott/scriptling/object"
)

// GetMCPLibrary returns the MCP helper library for scriptling (used by MCP tool scripts running within the same server)
// This provides:
// - Parameter access functions (get, return_*) for MCP tool scripts
// - Tool access functions that use direct MCP server access (no API calls)
func GetMCPLibrary(mcpParams map[string]string, openaiClient *openai.Client) *object.Library {
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
		"list_tools": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpListToolsMCP(ctx, openaiClient, kwargs, args...)
			},
			HelpText: "list_tools() - Get list of available MCP tools and their parameters.",
		},
		"call_tool": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpCallToolMCP(ctx, openaiClient, kwargs, args...)
			},
			HelpText: "call_tool(name, arguments) - Call an MCP tool directly. Arguments should be a dict.",
		},
		"tool_search": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpToolSearchMCP(ctx, openaiClient, kwargs, args...)
			},
			HelpText: "tool_search(query[, namespace]) - Search for tools by keyword. Returns list of matching tools.",
		},
		"execute_tool": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpExecuteToolMCP(ctx, openaiClient, kwargs, args...)
			},
			HelpText: "execute_tool(name, arguments[, namespace]) - Execute a discovered tool. Arguments should be a dict.",
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

// mcpListToolsMCP fetches available tools directly from MCP server
func mcpListToolsMCP(ctx context.Context, openaiClient *openai.Client, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if openaiClient == nil {
		return &object.Error{Message: "MCP tools not available - OpenAI client not configured"}
	}

	// Create independent context for tool listing
	mcpCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Copy user from original context if it exists
	if user := ctx.Value("user"); user != nil {
		mcpCtx = context.WithValue(mcpCtx, "user", user)
	}

	// Get tools from MCP server
	mcpTools := openaiClient.GetMCPServer().ListTools()

	// Convert to scriptling objects
	toolList := make([]object.Object, 0, len(mcpTools))
	for _, tool := range mcpTools {
		toolDict := &object.Dict{
			Pairs: map[string]object.DictPair{
				"name": {
					Key:   &object.String{Value: "name"},
					Value: &object.String{Value: tool.Name},
				},
				"description": {
					Key:   &object.String{Value: "description"},
					Value: &object.String{Value: tool.Description},
				},
				"parameters": {
					Key:   &object.String{Value: "parameters"},
					Value: convertToScriptlingObject(tool.InputSchema),
				},
			},
		}
		toolList = append(toolList, toolDict)
	}

	return &object.List{Elements: toolList}
}

// mcpCallToolMCP calls a tool directly on MCP server
func mcpCallToolMCP(ctx context.Context, openaiClient *openai.Client, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "call_tool() requires tool name and arguments"}
	}

	if openaiClient == nil {
		return &object.Error{Message: "MCP tools not available - OpenAI client not configured"}
	}

	// Get tool name
	toolNameStr, ok := args[0].(*object.String)
	if !ok {
		return &object.Error{Message: "call_tool() first argument must be a string (tool name)"}
	}
	toolName := toolNameStr.Value

	// Get arguments
	argsDict, ok := args[1].(*object.Dict)
	if !ok {
		return &object.Error{Message: "call_tool() second argument must be a dict (arguments)"}
	}

	// Convert arguments to map
	arguments := make(map[string]any)
	for _, pair := range argsDict.Pairs {
		key := pair.Key.(*object.String).Value
		arguments[key] = convertFromScriptlingObject(pair.Value)
	}

	// Create independent context for tool call
	mcpCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Copy user from original context if it exists
	if user := ctx.Value("user"); user != nil {
		mcpCtx = context.WithValue(mcpCtx, "user", user)
	}

	// Call tool on MCP server
	response, err := openaiClient.GetMCPServer().CallTool(mcpCtx, toolName, arguments)
	if err != nil {
		errMsg := fmt.Sprintf("Tool call failed: %v", err)
		return &object.Error{Message: errMsg}
	}

	return convertToScriptlingObject(response.Content)
}

// mcpToolSearchMCP searches for tools by keyword on MCP server
func mcpToolSearchMCP(ctx context.Context, openaiClient *openai.Client, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "tool_search() requires a search query"}
	}

	if openaiClient == nil {
		return &object.Error{Message: "MCP tools not available - OpenAI client not configured"}
	}

	// Get search query
	queryStr, ok := args[0].(*object.String)
	if !ok {
		return &object.Error{Message: "tool_search() first argument must be a string (search query)"}
	}

	// Get optional namespace
	namespace := ""
	if len(args) >= 2 {
		if nsStr, ok := args[1].(*object.String); ok {
			namespace = nsStr.Value
		}
	}

	// Build tool name with namespace prefix if provided
	toolName := "tool_search"
	if namespace != "" {
		toolName = namespace + "/" + toolName
	}

	// Use call_tool to call tool_search
	searchArgs := &object.Dict{
		Pairs: map[string]object.DictPair{
			"query": {
				Key:   &object.String{Value: "query"},
				Value: &object.String{Value: queryStr.Value},
			},
		},
	}

	return mcpCallToolMCP(ctx, openaiClient, kwargs, &object.String{Value: toolName}, searchArgs)
}

// mcpExecuteToolMCP executes a discovered tool on MCP server
func mcpExecuteToolMCP(ctx context.Context, openaiClient *openai.Client, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "execute_tool() requires tool name and arguments"}
	}

	if openaiClient == nil {
		return &object.Error{Message: "MCP tools not available - OpenAI client not configured"}
	}

	// Get tool name
	toolNameStr, ok := args[0].(*object.String)
	if !ok {
		return &object.Error{Message: "execute_tool() first argument must be a string (tool name)"}
	}

	// Get arguments
	argsDict, ok := args[1].(*object.Dict)
	if !ok {
		return &object.Error{Message: "execute_tool() second argument must be a dict (arguments)"}
	}

	// Get optional namespace
	namespace := ""
	if len(args) >= 3 {
		if nsStr, ok := args[2].(*object.String); ok {
			namespace = nsStr.Value
		}
	}

	// Convert arguments to JSON string
	arguments := make(map[string]interface{})
	for _, pair := range argsDict.Pairs {
		key := pair.Key.(*object.String).Value
		arguments[key] = convertFromScriptlingObject(pair.Value)
	}

	argumentsJSON, err := json.Marshal(arguments)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to marshal arguments: %v", err)}
	}

	// Build tool name with namespace prefix if provided
	executeToolName := "execute_tool"
	if namespace != "" {
		executeToolName = namespace + "/" + executeToolName
	}

	// Use call_tool to call execute_tool
	executeArgs := &object.Dict{
		Pairs: map[string]object.DictPair{
			"name": {
				Key:   &object.String{Value: "name"},
				Value: &object.String{Value: toolNameStr.Value},
			},
			"arguments": {
				Key:   &object.String{Value: "arguments"},
				Value: &object.String{Value: string(argumentsJSON)},
			},
		},
	}

	return mcpCallToolMCP(ctx, openaiClient, kwargs, &object.String{Value: executeToolName}, executeArgs)
}