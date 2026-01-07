package scriptling

import (
	"context"
	"fmt"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/mcp"
	"github.com/paularlott/mcp/toon"
	scriptlib "github.com/paularlott/scriptling"
	scriptlingmcp "github.com/paularlott/scriptling/mcp"
	"github.com/paularlott/scriptling/object"
)

// GetMCPToolsLibrary returns the MCP tools library for scriptling (used in local/remote environments)
// This provides only tool access functions that communicate with the server via API calls
func GetMCPToolsLibrary(client *apiclient.ApiClient) *object.Library {
	functions := map[string]*object.Builtin{
		"list_tools": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpListTools(ctx, client, kwargs, args...)
			},
			HelpText: "list_tools() - Get list of available MCP tools and their parameters.",
		},
		"call_tool": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpCallTool(ctx, client, kwargs, args...)
			},
			HelpText: "call_tool(name, arguments) - Call an MCP tool directly. Arguments should be a dict.",
		},
		"tool_search": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpToolSearch(ctx, client, kwargs, args...)
			},
			HelpText: "tool_search(query) - Search for tools by keyword. Returns list of matching tools with names like 'namespace/toolname'.",
		},
		"execute_tool": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpExecuteTool(ctx, client, kwargs, args...)
			},
			HelpText: "execute_tool(name, arguments) - Execute a discovered tool. Use full name like 'namespace/toolname' for namespaced tools. Arguments should be a dict.",
		},
		"toon_encode": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
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
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
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

	return object.NewLibrary(functions, nil, "MCP tool functions")
}

// mcpListTools fetches available tools via API for local/remote environments
func mcpListTools(ctx context.Context, client *apiclient.ApiClient, kwargs map[string]object.Object, args ...object.Object) object.Object {
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
func mcpCallTool(ctx context.Context, client *apiclient.ApiClient, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "call_tool() requires tool name and arguments"}
	}

	if client == nil {
		return &object.Error{Message: "MCP tools not available - API client not configured"}
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
	arguments := make(map[string]interface{})
	for _, pair := range argsDict.Pairs {
		key := pair.Key.(*object.String).Value
		arguments[key] = scriptlib.ToGo(pair.Value)
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
	_, err := client.Do(mcpCtx, "POST", "api/chat/tools/call", req, &response)
	if err != nil {
		errMsg := fmt.Sprintf("Tool call failed: %v", err)
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
	queryStr, ok := args[0].(*object.String)
	if !ok {
		return &object.Error{Message: "tool_search() first argument must be a string (search query)"}
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

	result := mcpCallTool(ctx, client, kwargs, &object.String{Value: "tool_search"}, searchArgs)

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
func mcpExecuteTool(ctx context.Context, client *apiclient.ApiClient, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "execute_tool() requires tool name and arguments"}
	}

	if client == nil {
		return &object.Error{Message: "MCP tools not available - API client not configured"}
	}

	// Get tool name (can be namespaced like "namespace/toolname")
	toolNameStr, ok := args[0].(*object.String)
	if !ok {
		return &object.Error{Message: "execute_tool() first argument must be a string (tool name)"}
	}

	// Get arguments
	argsDict, ok := args[1].(*object.Dict)
	if !ok {
		return &object.Error{Message: "execute_tool() second argument must be a dict (arguments)"}
	}

	// Convert arguments to map[string]interface{} for the execute_tool call
	arguments := make(map[string]interface{})
	for _, pair := range argsDict.Pairs {
		key := pair.Key.(*object.String).Value
		arguments[key] = scriptlib.ToGo(pair.Value)
	}

	// Create request for execute_tool
	req := mcp.ToolCallParams{
		Name: "execute_tool",
		Arguments: map[string]interface{}{
			"name":      toolNameStr.Value,
			"arguments": arguments,
		},
	}

	// Create independent context for tool call to prevent script timeout
	mcpCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Call API to execute tool
	var response *mcp.ToolResponse
	_, err := client.Do(mcpCtx, "POST", "api/chat/tools/call", req, &response)
	if err != nil {
		errMsg := fmt.Sprintf("Tool execution failed: %v", err)
		return &object.Error{Message: errMsg}
	}

	// Use the bridge package to decode the response
	return scriptlingmcp.DecodeToolResponse(response)
}
