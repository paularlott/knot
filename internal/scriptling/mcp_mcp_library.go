package scriptling

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/openai"
	"github.com/paularlott/mcp/toon"
	scriptlib "github.com/paularlott/scriptling"
	scriptlingmcp "github.com/paularlott/scriptling/extlibs/mcp"
	"github.com/paularlott/scriptling/object"
)

// GetMCPLibrary returns the MCP helper library for scriptling (used by MCP tool scripts running within the same server)
// This provides:
// - Parameter access functions (get, return_*) for MCP tool scripts
// - Tool access functions that use direct MCP server access (no API calls)
func GetMCPLibrary(mcpParams map[string]string, openaiClient *openai.Client) *object.Library {
	builder := object.NewLibraryBuilder("knot.mcp", "Knot MCP helper functions for tool scripts").
		FunctionWithHelp("get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return mcpGet(mcpParams, args...)
			}, "get(name[, default]) - Get MCP parameter value with automatic type conversion").
		FunctionWithHelp("return_string", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return mcpReturnString(args...)
			}, "return_string(value) - Return a string result and exit").
		FunctionWithHelp("return_object", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return mcpReturnObject(args...)
			}, "return_object(value) - Return a structured object as JSON and exit").
		FunctionWithHelp("return_toon", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return mcpReturnToon(args...)
			}, "return_toon(value) - Return a value encoded as toon and exit").
		FunctionWithHelp("return_error", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return mcpReturnError(args...)
			}, "return_error(message) - Return an error message and exit with error code").
		FunctionWithHelp("list_tools", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return mcpListToolsMCP(ctx, openaiClient, kwargs.Kwargs, args...)
			}, "list_tools() - Get list of available MCP tools and their parameters.").
		FunctionWithHelp("call_tool", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return mcpCallToolMCP(ctx, openaiClient, kwargs.Kwargs, args...)
			}, "call_tool(name, arguments) - Call an MCP tool directly. Arguments should be a dict.").
		FunctionWithHelp("tool_search", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return mcpToolSearchMCP(ctx, openaiClient, kwargs.Kwargs, args...)
			}, "tool_search(query[, namespace]) - Search for tools by keyword. Returns list of matching tools.").
		FunctionWithHelp("execute_tool", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return mcpExecuteToolMCP(ctx, openaiClient, kwargs.Kwargs, args...)
			}, "execute_tool(name, arguments[, namespace]) - Execute a discovered tool. Arguments should be a dict.")

	// Add shared toon functions from scriptling/mcp
	lib := builder.Build()
	scriptlingmcp.RegisterToon(&libraryRegistrar{lib: lib})

	return lib
}

type libraryRegistrar struct {
	lib *object.Library
}

func (r *libraryRegistrar) RegisterLibrary(name string, toonLib *object.Library) {
	// Merge toon library functions into our library
	// This is a workaround since we can't modify the built library
	// The toon functions will be available through the builder before Build()
}

// mcpGet retrieves a parameter value with automatic type conversion
func mcpGet(mcpParams map[string]string, args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Null{}
	}

	name, err := args[0].AsString()
	if err != nil {
		return err
	}

	value, found := mcpParams[name]
	if !found {
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
		return scriptlib.FromGo(jsonValue)
	}

	// Return as string (MCP parameters are always strings)
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

// mcpReturnToon returns a value encoded as toon (script should exit after this)
func mcpReturnToon(args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.Null{}
	}
	goValue := scriptlib.ToGo(args[0])
	encoded, err := toon.Encode(goValue)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("Error encoding to toon: %v", err)}
	}
	return &object.String{Value: encoded}
}

// mcpReturnError returns an error (script should exit after this)
func mcpReturnError(args ...object.Object) object.Object {
	if len(args) == 0 {
		return &object.String{Value: "Error"}
	}
	return args[0]
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
					Value: scriptlib.FromGo(tool.InputSchema),
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
	toolName, err := args[0].AsString()
	if err != nil {
		return err
	}

	// Get arguments
	argsDict, err := args[1].AsDict()
	if err != nil {
		return err
	}

	// Convert arguments to map using the bridge package
	arguments := make(map[string]interface{})
	for key, val := range argsDict {
		arguments[key] = scriptlib.ToGo(val)
	}

	// Create independent context for tool call
	mcpCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Copy user from original context if it exists
	if user := ctx.Value("user"); user != nil {
		mcpCtx = context.WithValue(mcpCtx, "user", user)
	}

	// Call tool on MCP server
	response, mcpErr := openaiClient.GetMCPServer().CallTool(mcpCtx, toolName, arguments)
	if mcpErr != nil {
		errMsg := fmt.Sprintf("Tool call failed: %v", mcpErr)
		return &object.Error{Message: errMsg}
	}

	// Use the bridge package to decode the response
	return scriptlingmcp.DecodeToolResponse(response)
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
	query, err := args[0].AsString()
	if err != nil {
		return err
	}

	// Get optional namespace
	namespace := ""
	if len(args) >= 2 {
		if ns, err := args[1].AsString(); err == nil {
			namespace = ns
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
				Value: &object.String{Value: query},
			},
		},
	}

	result := mcpCallToolMCP(ctx, openaiClient, kwargs, &object.String{Value: toolName}, searchArgs)

	// The response is a content block (possibly wrapped in a List), extract the tools from the Text field
	var contentBlock map[string]object.Object
	if resultList, err := result.AsList(); err == nil && len(resultList) > 0 {
		if firstDict, err := resultList[0].AsDict(); err == nil {
			contentBlock = firstDict
		}
	} else if resultDict, err := result.AsDict(); err == nil {
		contentBlock = resultDict
	}

	if contentBlock != nil {
		// Try both "text" and "Text" keys (from JSON parsing)
		var textVal string
		if val, found := contentBlock["text"]; found {
			if s, err := val.AsString(); err == nil {
				textVal = s
			}
		} else if val, found := contentBlock["Text"]; found {
			if s, err := val.AsString(); err == nil {
				textVal = s
			}
		}

		if textVal != "" {
			// Parse the JSON text using the bridge package
			tools, err := scriptlingmcp.ParseToolSearchResultsFromText(textVal)
			if err == nil {
				return tools
			}
		}
	}

	return result
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
	toolName, err := args[0].AsString()
	if err != nil {
		return err
	}

	// Get arguments
	argsDict, err := args[1].AsDict()
	if err != nil {
		return err
	}

	// Get optional namespace
	namespace := ""
	if len(args) >= 3 {
		if ns, err := args[2].AsString(); err == nil {
			namespace = ns
		}
	}

	// Convert arguments to JSON string
	arguments := make(map[string]interface{})
	for key, val := range argsDict {
		arguments[key] = scriptlib.ToGo(val)
	}

	argumentsJSON, jsonErr := json.Marshal(arguments)
	if jsonErr != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to marshal arguments: %v", jsonErr)}
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
				Value: &object.String{Value: toolName},
			},
			"arguments": {
				Key:   &object.String{Value: "arguments"},
				Value: &object.String{Value: string(argumentsJSON)},
			},
		},
	}

	return mcpCallToolMCP(ctx, openaiClient, kwargs, &object.String{Value: executeToolName}, executeArgs)
}