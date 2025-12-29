package scriptling

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/paularlott/knot/apiclient"
	scriptlib "github.com/paularlott/scriptling"
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
					Value: scriptlib.FromGo(tool.Parameters),
				},
			},
		}
		toolList = append(toolList, toolDict)
	}

	return &object.List{Elements: toolList}
}

// mcpCallTool calls a tool via API for local/remote environments
// decodeToolResponse intelligently decodes a tool response for easier use in scripts
// - Single text content: returns the text as a string
// - Text that is valid JSON: returns the parsed JSON as objects
// - Multiple content blocks: returns the list of decoded blocks
// - Structured content: returns the decoded structure
func decodeToolResponse(response interface{}) object.Object {
	// Handle if response is already a simple type
	switch v := response.(type) {
	case string:
		return decodeToolText(v)
	case map[string]interface{}:
		// Single content block (not wrapped in array)
		return decodeToolContent(v)
	case []interface{}:
		// Empty content
		if len(v) == 0 {
			return &object.Null{}
		}
		// Single content block
		if len(v) == 1 {
			return decodeToolContent(v[0])
		}
		// Multiple content blocks - decode each
		elements := make([]object.Object, len(v))
		for i, block := range v {
			elements[i] = decodeToolContent(block)
		}
		return &object.List{Elements: elements}
	default:
		// Fallback to normal conversion
		return scriptlib.FromGo(response)
	}
}

// decodeToolContent decodes a single content block
func decodeToolContent(block interface{}) object.Object {
	contentMap, ok := block.(map[string]interface{})
	if !ok {
		return scriptlib.FromGo(block)
	}

	// Get content type - keys from JSON are capitalized (Type, Text, etc.)
	contentType := ""
	if v, ok := contentMap["Type"].(string); ok {
		contentType = v
	} else if v, ok := contentMap["type"].(string); ok {
		contentType = v
	}

	switch contentType {
	case "text":
		// Try both capitalized and lowercase keys for text field
		var text string
		if v, ok := contentMap["Text"].(string); ok {
			text = v
		} else if v, ok := contentMap["text"].(string); ok {
			text = v
		}
		if text != "" {
			return decodeToolText(text)
		}
	case "image":
		// Return image block with data and mimeType
		return scriptlib.FromGo(contentMap)
	case "resource":
		// Return resource block
		return scriptlib.FromGo(contentMap)
	default:
		// Unknown type, return as-is
		return scriptlib.FromGo(contentMap)
	}

	return scriptlib.FromGo(block)
}

// decodeToolText decodes text content, parsing JSON if valid
func decodeToolText(text string) object.Object {
	// Try to parse as JSON
	var jsonValue interface{}
	if err := json.Unmarshal([]byte(text), &jsonValue); err == nil {
		return scriptlib.FromGo(jsonValue)
	}
	// Return as plain string
	return &object.String{Value: text}
}

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

	return decodeToolResponse(response.Content)
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
								Value: scriptlib.FromGo(v),
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
	// Note: execute_tool expects arguments as a map/object, not as JSON string
	req := ToolCallRequest{
		Name:      "execute_tool",
		Arguments: map[string]interface{}{
			"name":      toolNameStr.Value,
			"arguments": arguments,
		},
	}

	// Create independent context for tool call to prevent script timeout
	mcpCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Call API to execute tool
	var response ToolCallResponse
	_, err := client.Do(mcpCtx, "POST", "api/chat/tools/call", req, &response)
	if err != nil {
		errMsg := fmt.Sprintf("Tool execution failed: %v", err)
		return &object.Error{Message: errMsg}
	}

	return decodeToolResponse(response.Content)
}