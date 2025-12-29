package scriptling

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/paularlott/knot/apiclient"
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
			HelpText: "tool_search(query[, namespace]) - Search for tools by keyword. Returns list of matching tools.",
		},
		"execute_tool": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return mcpExecuteTool(ctx, client, kwargs, args...)
			},
			HelpText: "execute_tool(name, arguments[, namespace]) - Execute a discovered tool. Arguments should be a dict.",
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
	var tools []Tool
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
					Value: convertToScriptlingObject(tool.Parameters),
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
		arguments[key] = convertFromScriptlingObject(pair.Value)
	}

	// Create request
	req := ToolCallRequest{
		Name:      toolName,
		Arguments: arguments,
	}

	// Create independent context for tool call to prevent script timeout
	mcpCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Call API to execute tool
	var response ToolCallResponse
	_, err := client.Do(mcpCtx, "POST", "api/chat/tools/call", req, &response)
	if err != nil {
		errMsg := fmt.Sprintf("Tool call failed: %v", err)
		return &object.Error{Message: errMsg}
	}

	return convertToScriptlingObject(response.Content)
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

	result := mcpCallTool(ctx, client, kwargs, &object.String{Value: toolName}, searchArgs)

	// The response is a content block (possibly wrapped in a List), extract the tools from the Text field
	var contentBlock *object.Dict
	if resultList, ok := result.(*object.List); ok && len(resultList.Elements) > 0 {
		if firstDict, ok := resultList.Elements[0].(*object.Dict); ok {
			contentBlock = firstDict
		}
	} else if resultDict, ok := result.(*object.Dict); ok {
		contentBlock = resultDict
	}

	if contentBlock != nil {
		if textVal, found := contentBlock.Pairs["Text"]; found {
			if textStr, ok := textVal.Value.(*object.String); ok {
				// Parse the JSON in the Text field
				var tools []map[string]interface{}
				if err := json.Unmarshal([]byte(textStr.Value), &tools); err == nil {
					// Convert to scriptling objects, same format as list_tools
					toolList := make([]object.Object, 0, len(tools))
					for _, tool := range tools {
						toolDict := &object.Dict{
							Pairs: map[string]object.DictPair{},
						}
						for k, v := range tool {
							toolDict.Pairs[k] = object.DictPair{
								Key:   &object.String{Value: k},
								Value: convertToScriptlingObject(v),
							}
						}
						toolList = append(toolList, toolDict)
					}
					return &object.List{Elements: toolList}
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

	return mcpCallTool(ctx, client, kwargs, &object.String{Value: executeToolName}, executeArgs)
}