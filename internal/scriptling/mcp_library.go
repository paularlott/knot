package scriptling

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/mcp"
	mcptoon "github.com/paularlott/mcp/toon"
	scriptlib "github.com/paularlott/scriptling"
	scriptlingmcp "github.com/paularlott/scriptling/extlibs/mcp"
	"github.com/paularlott/scriptling/object"
)

// GetMCPToolsLibrary returns the MCP tools library for scriptling (used in local/remote environments)
// This provides only tool access functions that communicate with the server via API calls
func GetMCPToolsLibrary(client *apiclient.ApiClient, mcpParams map[string]string) *object.Library {
	builder := object.NewLibraryBuilder("knot.mcp", "Knot MCP tool functions")

	if mcpParams != nil {
		builder.FunctionWithHelp("get_int", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
			return mcpGetInt(mcpParams, args...)
		}, `get_int(name, default=0) - Get a parameter as integer

Safely gets a parameter and converts it to an integer, handling None, empty strings, and whitespace.
Returns the default value if the parameter doesn't exist, is None, empty, or whitespace-only.

Parameters:
  name: The parameter name
  default: Default value (optional, defaults to 0)

Example:
  project_id = mcp.get_int("project_id", 0)
  limit = mcp.get_int("limit", 100)`)

		builder.FunctionWithHelp("get_float", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
			return mcpGetFloat(mcpParams, args...)
		}, `get_float(name, default=0.0) - Get a parameter as float

Safely gets a parameter and converts it to a float, handling None, empty strings, and whitespace.
Returns the default value if the parameter doesn't exist, is None, empty, or whitespace-only.

Parameters:
  name: The parameter name
  default: Default value (optional, defaults to 0.0)

Example:
  price = mcp.get_float("price", 0.0)
  percentage = mcp.get_float("percentage", 100.0)`)

		builder.FunctionWithHelp("get_string", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
			return mcpGetString(mcpParams, args...)
		}, `get_string(name, default="") - Get a parameter as string

Safely gets a parameter as a string, handling None, empty strings, and whitespace.
Trims whitespace and returns the default value if the parameter doesn't exist, is None, empty, or whitespace-only.

Parameters:
  name: The parameter name
  default: Default value (optional, defaults to empty string)

Example:
  status = mcp.get_string("status", "pending")
  format = mcp.get_string("format", "json")`)

		builder.FunctionWithHelp("get_bool", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
			return mcpGetBool(mcpParams, args...)
		}, `get_bool(name, default=False) - Get a parameter as boolean

Safely gets a parameter as a boolean, handling various string representations.
Returns the default value if the parameter doesn't exist, is None, empty, or invalid.

Accepts: true/false, yes/no, 1/0, on/off, enabled/disabled (case-insensitive)

Parameters:
  name: The parameter name
  default: Default value (optional, defaults to False)

Example:
  include_archived = mcp.get_bool("include_archived", False)
  is_active = mcp.get_bool("is_active", True)`)

		builder.FunctionWithHelp("get_list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
			return mcpGetList(mcpParams, args...)
		}, `get_list(name, default=[]) - Get a parameter as list

Safely gets a parameter as a list, handling comma-separated strings or arrays.
Returns the default value if the parameter doesn't exist or is None.

For comma-separated strings, splits by comma and trims whitespace from each item.
Empty items are filtered out.

Parameters:
  name: The parameter name
  default: Default value (optional, defaults to empty list)

Example:
  ids = mcp.get_list("ids")              # "1,2,3" → ["1", "2", "3"]
  tags = mcp.get_list("tags", ["all"])   # "tag1, tag2" → ["tag1", "tag2"]
  filters = mcp.get_list("filters")      # Already array → unchanged`)

		builder.FunctionWithHelp("return_string", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
			return mcpReturnString(args...)
		}, `return_string(text) - Return a string result from the tool and stop execution

Sets the tool's return value to the given string and stops script execution.
No code after this call will execute.

Example:
  mcp.return_string("Search completed successfully")
  # Code here will not execute`)

		builder.FunctionWithHelp("return_object", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
			return mcpReturnObject(args...)
		}, `return_object(obj) - Return an object as JSON from the tool and stop execution

Serializes the object to JSON and sets it as the tool's return value.
Stops script execution immediately - no code after this call will execute.

Example:
  mcp.return_object({"status": "success", "count": 42})
  # Code here will not execute`)

		builder.FunctionWithHelp("return_toon", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
			return mcpReturnToon(args...)
		}, `return_toon(obj) - Return an object encoded as TOON from the tool and stop execution

Serializes the object to TOON format and sets it as the tool's return value.
TOON is a compact text format optimized for LLM consumption.
Stops script execution immediately - no code after this call will execute.

Example:
  mcp.return_toon({"result": data})
  # Code here will not execute`)

		builder.FunctionWithHelp("return_error", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
			return mcpReturnError(args...)
		}, `return_error(message) - Return an error from the tool and stop execution

Returns an error message to the MCP client and stops script execution immediately.

Arguments:
  message (str): The error message

Example:
  mcp.return_error("Customer not found")
  mcp.return_error("Invalid input: project ID is required")`)
	}

	builder.FunctionWithHelp("list_tools", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return mcpListTools(ctx, client, args...)
	}, "list_tools() - Get list of available MCP tools and their parameters.")

	builder.FunctionWithHelp("call_tool", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return mcpCallTool(ctx, client, args...)
	}, "call_tool(name, arguments) - Call an MCP tool directly. Arguments should be a dict.")

	builder.FunctionWithHelp("tool_search", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return mcpToolSearch(ctx, client, kwargs.Kwargs, args...)
	}, "tool_search(query, max_results=10) - Search for tools by keyword. Returns list of matching tools.")

	builder.FunctionWithHelp("execute_tool", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return mcpExecuteTool(ctx, client, args...)
	}, "execute_tool(name, arguments) - Execute a discovered tool. Arguments should be a dict.")

	return builder.Build()
}

// mcpListTools fetches available tools via API for local/remote environments
func mcpListTools(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if client == nil {
		return &object.Error{Message: "MCP tools not available - API client not configured"}
	}

	// Create independent context for tool listing to prevent script timeout
	mcpCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Call API to get tools
	var tools []mcp.MCPTool
	_, err := client.Do(mcpCtx, "GET", "api/chat/tools", nil, &tools)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to list tools: %v", err)
		return &object.Error{Message: errMsg}
	}

	// Convert to scriptling objects
	toolList := make([]object.Object, 0, len(tools))
	for _, tool := range tools {
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
					Value: scriptlib.FromGo(tool.InputSchema),
				},
			},
		}
		toolList = append(toolList, toolDict)
	}

	return &object.List{Elements: toolList}
}

// mcpCallTool calls a tool via API for local/remote environments
func mcpCallTool(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "call_tool() requires tool name and arguments"}
	}

	if client == nil {
		return &object.Error{Message: "MCP tools not available - API client not configured"}
	}

	// Get tool name
	toolName, err := args[0].AsString()
	if err != nil {
		return err
	}

	// Get arguments
	argsDict, err := args[1].AsDict()
	if err != nil {
		return &object.Error{Message: "call_tool() second argument must be a dict (arguments)"}
	}

	// Convert arguments to map
	arguments := make(map[string]interface{})
	for key, val := range argsDict {
		arguments[key] = scriptlib.ToGo(val)
	}

	// Create request
	req := mcp.ToolCallParams{
		Name:      toolName,
		Arguments: arguments,
	}

	// Create independent context for tool call to prevent script timeout
	mcpCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Call API to execute tool
	var response *mcp.ToolResponse
	_, apiErr := client.DoJSON(mcpCtx, "POST", "api/chat/tools/call", req, &response)
	if apiErr != nil {
		errMsg := fmt.Sprintf("Tool call failed: %v", apiErr)
		return &object.Error{Message: errMsg}
	}

	// Use the bridge package to decode the response
	return scriptlingmcp.DecodeToolResponse(response)
}

// mcpToolSearch searches for tools by keyword via API
func mcpToolSearch(ctx context.Context, client *apiclient.ApiClient, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "tool_search() requires a search query"}
	}

	if client == nil {
		return &object.Error{Message: "MCP tools not available - API client not configured"}
	}

	// Get search query
	query, err := args[0].AsString()
	if err != nil {
		return err
	}

	// Get optional max_results from kwargs (default: 10)
	maxResults := 10
	if maxResultsObj, found := kwargs["max_results"]; found {
		if maxResultsInt, err := maxResultsObj.AsInt(); err == nil {
			maxResults = int(maxResultsInt)
		}
	}

	// Use call_tool to call tool_search
	searchArgs := &object.Dict{
		Pairs: map[string]object.DictPair{
			"query": {
				Key:   &object.String{Value: "query"},
				Value: &object.String{Value: query},
			},
			"max_results": {
				Key:   &object.String{Value: "max_results"},
				Value: &object.Integer{Value: int64(maxResults)},
			},
		},
	}

	result := mcpCallTool(ctx, client, &object.String{Value: "tool_search"}, searchArgs)

	// The result should be a scriptling object from the bridge package
	// If it's a list with a single dict containing "text" field, parse the tools
	if resultList, ok := result.(*object.List); ok && len(resultList.Elements) == 1 {
		if resultDict, ok := resultList.Elements[0].(*object.Dict); ok {
			// Try both "text" and "Text" keys (from JSON parsing)
			var textVal *object.String
			if val, found := resultDict.Pairs["text"]; found {
				if s, ok := val.Value.(*object.String); ok {
					textVal = s
				}
			} else if val, found := resultDict.Pairs["Text"]; found {
				if s, ok := val.Value.(*object.String); ok {
					textVal = s
				}
			}

			if textVal != nil {
				// Parse the JSON text to extract tools using the bridge package
				tools, err := scriptlingmcp.ParseToolSearchResultsFromText(textVal.Value)
				if err == nil {
					return tools
				}
			}
		}
	}

	return result
}

// mcpExecuteTool executes a discovered tool via API
func mcpExecuteTool(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "execute_tool() requires tool name and arguments"}
	}

	if client == nil {
		return &object.Error{Message: "MCP tools not available - API client not configured"}
	}

	// Get tool name
	toolName, err := args[0].AsString()
	if err != nil {
		return err
	}

	// Get arguments
	argsDict, err := args[1].AsDict()
	if err != nil {
		return &object.Error{Message: "execute_tool() second argument must be a dict (arguments)"}
	}

	// Convert arguments to map[string]interface{} for the execute_tool call
	arguments := make(map[string]interface{})
	for key, val := range argsDict {
		arguments[key] = scriptlib.ToGo(val)
	}

	// Create request for execute_tool
	req := mcp.ToolCallParams{
		Name: "execute_tool",
		Arguments: map[string]interface{}{
			"name":      toolName,
			"arguments": arguments,
		},
	}

	// Create independent context for tool call to prevent script timeout
	mcpCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Call API to execute tool
	var response *mcp.ToolResponse
	_, apiErr := client.DoJSON(mcpCtx, "POST", "api/chat/tools/call", req, &response)
	if apiErr != nil {
		errMsg := fmt.Sprintf("Tool execution failed: %v", apiErr)
		return &object.Error{Message: errMsg}
	}

	// Use the bridge package to decode the response
	return scriptlingmcp.DecodeToolResponse(response)
}

// mcpGetFloat retrieves a parameter value as a float with safe handling of None, empty strings, and whitespace
func mcpGetFloat(mcpParams map[string]string, args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Float{Value: 0.0}
	}

	name, err := args[0].AsString()
	if err != nil {
		return err
	}

	// Get default value (default to 0.0)
	var defaultFloat float64 = 0.0
	if len(args) >= 2 {
		defVal, _ := args[1].CoerceFloat()
		defaultFloat = defVal
	}

	value, exists := mcpParams[name]
	if !exists || value == "" {
		return &object.Float{Value: defaultFloat}
	}

	// Convert to scriptling object
	obj := scriptlib.FromGo(value)

	// Use CoerceFloat for automatic conversion
	floatVal, errObj := obj.CoerceFloat()
	if errObj != nil {
		// Return default on conversion error
		return &object.Float{Value: defaultFloat}
	}
	return &object.Float{Value: floatVal}
}

// mcpGetBool retrieves a parameter value as a boolean with safe handling of various string representations
func mcpGetBool(mcpParams map[string]string, args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Boolean{Value: false}
	}

	name, err := args[0].AsString()
	if err != nil {
		return err
	}

	// Get default value (default to false)
	defaultBool := false
	if len(args) >= 2 {
		defaultBool, _ = args[1].AsBool()
	}

	value, exists := mcpParams[name]
	if !exists || value == "" {
		return &object.Boolean{Value: defaultBool}
	}

	// Convert to scriptling object
	obj := scriptlib.FromGo(value)

	// Handle strings - check for common boolean representations
	if strObj, ok := obj.(*object.String); ok {
		trimmed := strings.ToLower(strings.TrimSpace(strObj.Value))
		if trimmed == "" {
			return &object.Boolean{Value: defaultBool}
		}
		// Handle common boolean string representations
		switch trimmed {
		case "true", "yes", "1", "on", "enabled":
			return &object.Boolean{Value: true}
		case "false", "no", "0", "off", "disabled":
			return &object.Boolean{Value: false}
		default:
			// Invalid value, return default
			return &object.Boolean{Value: defaultBool}
		}
	}

	// Try direct boolean conversion
	boolVal, err := obj.AsBool()
	if err != nil {
		return &object.Boolean{Value: defaultBool}
	}
	return &object.Boolean{Value: boolVal}
}

// mcpGetList retrieves a parameter value as a list, handling comma-separated strings or arrays
func mcpGetList(mcpParams map[string]string, args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.List{Elements: []object.Object{}}
	}

	name, err := args[0].AsString()
	if err != nil {
		return err
	}

	// Get default value (default to empty list)
	var defaultList []object.Object
	if len(args) >= 2 {
		if listArg, ok := args[1].(*object.List); ok {
			defaultList = listArg.Elements
		} else {
			// Convert non-list to single-item list
			defaultList = []object.Object{args[1]}
		}
	}

	value, exists := mcpParams[name]
	if !exists || value == "" {
		return &object.List{Elements: defaultList}
	}

	// Convert to scriptling object
	obj := scriptlib.FromGo(value)

	// Handle strings - split by comma
	if strObj, ok := obj.(*object.String); ok {
		trimmed := strings.TrimSpace(strObj.Value)
		if trimmed == "" {
			return &object.List{Elements: defaultList}
		}

		// Split by comma and trim each item
		parts := strings.Split(trimmed, ",")
		result := make([]object.Object, 0, len(parts))
		for _, part := range parts {
			trimmedPart := strings.TrimSpace(part)
			if trimmedPart != "" {
				result = append(result, &object.String{Value: trimmedPart})
			}
		}
		return &object.List{Elements: result}
	}

	// Handle already lists
	if listObj, ok := obj.(*object.List); ok {
		return listObj
	}

	// Try list conversion
	listVal, err := obj.AsList()
	if err != nil {
		// If not a list, treat as single item
		return &object.List{Elements: []object.Object{obj}}
	}
	return &object.List{Elements: listVal}
}

// mcpGetInt retrieves a parameter value as an integer with safe handling of None, empty strings, and whitespace
func mcpGetInt(mcpParams map[string]string, args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Integer{Value: 0}
	}

	name, err := args[0].AsString()
	if err != nil {
		return err
	}

	// Get default value (default to 0)
	var defaultInt int64 = 0
	if len(args) >= 2 {
		defVal, _ := args[1].CoerceInt()
		defaultInt = defVal
	}

	value, exists := mcpParams[name]
	if !exists || value == "" {
		return &object.Integer{Value: defaultInt}
	}

	// Convert to scriptling object
	obj := scriptlib.FromGo(value)

	// Use CoerceInt which handles type conversion automatically
	intVal, errObj := obj.CoerceInt()
	if errObj != nil {
		// Return default on conversion error
		return &object.Integer{Value: defaultInt}
	}
	return &object.Integer{Value: intVal}
}

// mcpGetString retrieves a parameter value as a trimmed string with safe handling of None and whitespace
func mcpGetString(mcpParams map[string]string, args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.String{Value: ""}
	}

	name, err := args[0].AsString()
	if err != nil {
		return err
	}

	// Get default value (default to "")
	defaultValue := ""
	if len(args) >= 2 {
		defaultValue, _ = args[1].CoerceString()
	}

	value, exists := mcpParams[name]
	if !exists || value == "" {
		return &object.String{Value: defaultValue}
	}

	// Convert to scriptling object
	obj := scriptlib.FromGo(value)

	// Use CoerceString for automatic conversion, then trim
	strVal, _ := obj.CoerceString()
	trimmed := strings.TrimSpace(strVal)
	if trimmed == "" {
		return &object.String{Value: defaultValue}
	}
	return &object.String{Value: trimmed}
}

// mcpReturnString returns a string value and stops script execution
func mcpReturnString(args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Exception{Message: "SystemExit: 0", ExceptionType: "SystemExit"}
	}

	// Use CoerceString to allow integers and other types to be converted
	_, err := args[0].CoerceString()
	if err != nil {
		return err
	}

	// Use SystemExit to cleanly stop execution
	// SystemExit is handled specially by scriptling - it doesn't wrap in "Uncaught exception:"
	return &object.Exception{Message: "SystemExit: 0", ExceptionType: "SystemExit"}
}

// mcpReturnObject returns an object as JSON and stops script execution
func mcpReturnObject(args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Exception{Message: "SystemExit: 0", ExceptionType: "SystemExit"}
	}

	// Convert scriptling object to Go native types using scriptling's ToGo
	goObj := scriptlib.ToGo(args[0])

	// Marshal to JSON
	_, err := json.Marshal(goObj)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to serialize object to JSON: %v", err)}
	}

	// Use SystemExit to cleanly stop execution
	// SystemExit is handled specially by scriptling - it doesn't wrap in "Uncaught exception:"
	return &object.Exception{Message: "SystemExit: 0", ExceptionType: "SystemExit"}
}

// mcpReturnToon returns a value encoded as toon and stops script execution
func mcpReturnToon(args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Exception{Message: "SystemExit: 0", ExceptionType: "SystemExit"}
	}

	// Convert to Go object
	goObj := scriptlib.ToGo(args[0])

	_, err := mcptoon.Encode(goObj)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to encode to TOON: %v", err)}
	}

	// Use SystemExit to cleanly stop execution
	// SystemExit is handled specially by scriptling - it doesn't wrap in "Uncaught exception:"
	return &object.Exception{Message: "SystemExit: 0", ExceptionType: "SystemExit"}
}

// mcpReturnError returns an error and stops script execution
func mcpReturnError(args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Error{Message: "return_error requires a message argument"}
	}

	message, err := args[0].AsString()
	if err != nil {
		return err
	}

	// Store error message with prefix that executor.go can recognize
	errorResult := fmt.Sprintf("MCP_TOOL_ERROR: %s", message)

	// Return as Error which will stop execution
	return &object.Error{Message: errorResult}
}
