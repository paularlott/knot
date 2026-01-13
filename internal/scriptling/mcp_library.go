package scriptling

import (
	"context"
	"fmt"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/mcp"
	scriptlib "github.com/paularlott/scriptling"
	scriptlingmcp "github.com/paularlott/scriptling/mcp"
	"github.com/paularlott/scriptling/object"
)

// GetMCPToolsLibrary returns the MCP tools library for scriptling (used in local/remote environments)
// This provides only tool access functions that communicate with the server via API calls
func GetMCPToolsLibrary(client *apiclient.ApiClient) *object.Library {
	builder := object.NewLibraryBuilder("mcp", "MCP tool functions")

	builder.RawFunctionWithHelp("list_tools", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return mcpListTools(ctx, client, args...)
	}, "list_tools() - Get list of available MCP tools and their parameters.")

	builder.RawFunctionWithHelp("call_tool", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return mcpCallTool(ctx, client, args...)
	}, "call_tool(name, arguments) - Call an MCP tool directly. Arguments should be a dict.")

	builder.RawFunctionWithHelp("tool_search", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return mcpToolSearch(ctx, client, args...)
	}, "tool_search(query) - Search for tools by keyword. Returns list of matching tools with names like 'namespace/toolname'.")

	builder.RawFunctionWithHelp("execute_tool", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return mcpExecuteTool(ctx, client, args...)
	}, "execute_tool(name, arguments) - Execute a discovered tool. Use full name like 'namespace/toolname' for namespaced tools. Arguments should be a dict.")

	// Add shared toon functions from scriptling/mcp
	toonReg := &toonRegistrar{builder: builder}
	scriptlingmcp.RegisterToon(toonReg)

	return builder.Build()
}

type toonRegistrar struct {
	builder *object.LibraryBuilder
}

func (r *toonRegistrar) RegisterLibrary(name string, lib *object.Library) {
	funcs := lib.Functions()
	for fname, fn := range funcs {
		r.builder.RawFunctionWithHelp(name+"_"+fname, fn.Fn, fn.HelpText)
	}
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
	_, apiErr := client.Do(mcpCtx, "POST", "api/chat/tools/call", req, &response)
	if apiErr != nil {
		errMsg := fmt.Sprintf("Tool call failed: %v", apiErr)
		return &object.Error{Message: errMsg}
	}

	// Use the bridge package to decode the response
	return scriptlingmcp.DecodeToolResponse(response)
}

// mcpToolSearch searches for tools by keyword via API
func mcpToolSearch(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
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

	// Use call_tool to call tool_search
	searchArgs := &object.Dict{
		Pairs: map[string]object.DictPair{
			"query": {
				Key:   &object.String{Value: "query"},
				Value: &object.String{Value: query},
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

	// Get tool name (can be namespaced like "namespace/toolname")
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
	_, apiErr := client.Do(mcpCtx, "POST", "api/chat/tools/call", req, &response)
	if apiErr != nil {
		errMsg := fmt.Sprintf("Tool execution failed: %v", apiErr)
		return &object.Error{Message: errMsg}
	}

	// Use the bridge package to decode the response
	return scriptlingmcp.DecodeToolResponse(response)
}
