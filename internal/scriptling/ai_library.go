package scriptling

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/openai"
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

// GetAILibrary returns the AI helper library for scriptling (local/remote environments)
func GetAILibrary(client *apiclient.ApiClient, userId string) *object.Library {
	functions := map[string]*object.Builtin{
		"completion": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return aiCompletion(ctx, client, userId, kwargs, args...)
			},
			HelpText: "completion(messages) - Get AI completion from a list of messages. Each message should be a dict with 'role' and 'content' keys.",
		},
		"list_tools": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return aiListTools(ctx, client, kwargs, args...)
			},
			HelpText: "list_tools() - Get list of available tools and their parameters.",
		},
		"call_tool": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return aiCallTool(ctx, client, kwargs, args...)
			},
			HelpText: "call_tool(name, arguments) - Call a tool directly without AI. Arguments should be a dict.",
		},
	}

	return object.NewLibrary(functions, nil, "AI completion functions")
}

// GetAIMCPLibrary returns the AI helper library for MCP environment (uses MCP server directly)
func GetAIMCPLibrary(openaiClient *openai.Client) *object.Library {
	functions := map[string]*object.Builtin{
		"completion": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return aiCompletionMCP(ctx, openaiClient, kwargs, args...)
			},
			HelpText: "completion(messages) - Get AI completion from a list of messages. Each message should be a dict with 'role' and 'content' keys.",
		},
		"list_tools": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return aiListToolsMCP(ctx, openaiClient, kwargs, args...)
			},
			HelpText: "list_tools() - Get list of available MCP tools and their parameters.",
		},
		"call_tool": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return aiCallToolMCP(ctx, openaiClient, kwargs, args...)
			},
			HelpText: "call_tool(name, arguments) - Call an MCP tool directly without AI. Arguments should be a dict.",
		},
	}

	return object.NewLibrary(functions, nil, "AI completion functions")
}

func aiCompletion(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "completion() requires messages array"}
	}

	if client == nil {
		return &object.Error{Message: "AI completion not available - API client not configured"}
	}

	messagesList, ok := args[0].(*object.List)
	if !ok {
		return &object.Error{Message: "completion() first argument must be a list of messages"}
	}

	messages := make([]ChatMessage, 0, len(messagesList.Elements))
	for i, msgObj := range messagesList.Elements {
		msgDict, ok := msgObj.(*object.Dict)
		if !ok {
			return &object.Error{Message: fmt.Sprintf("message %d must be a dict with 'role' and 'content' keys", i)}
		}

		role, content := "", ""
		for _, pair := range msgDict.Pairs {
			key := pair.Key.(*object.String).Value
			if key == "role" {
				if roleStr, ok := pair.Value.(*object.String); ok {
					role = roleStr.Value
				}
			} else if key == "content" {
				if contentStr, ok := pair.Value.(*object.String); ok {
					content = contentStr.Value
				}
			}
		}

		if role == "" || content == "" {
			return &object.Error{Message: fmt.Sprintf("message %d missing 'role' or 'content' key", i)}
		}

		messages = append(messages, ChatMessage{
			Role:      role,
			Content:   content,
			Timestamp: time.Now().Unix(),
		})
	}

	// Create request
	req := ChatCompletionRequest{
		Messages: messages,
	}

	// Create independent context for AI completion to prevent script timeout from canceling AI operations
	aiCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Call API - the server will handle tool calling via MCP server integration
	var response ChatCompletionResponse
	_, err := client.Do(aiCtx, "POST", "api/chat/completion", req, &response)
	if err != nil {
		// Provide more helpful error message
		errMsg := fmt.Sprintf("AI completion failed: %v", err)
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
			errMsg += "\nNote: Make sure the server has AI chat enabled with valid OpenAI credentials"
		} else if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			errMsg += "\nNote: The server may not have the chat completion endpoint enabled"
		}
		return &object.Error{Message: errMsg}
	}

	return &object.String{Value: response.Content}
}

func aiCompletionMCP(ctx context.Context, openaiClient *openai.Client, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "completion() requires messages array"}
	}

	if openaiClient == nil {
		return &object.Error{Message: "AI completion not available - OpenAI client not configured"}
	}

	messagesList, ok := args[0].(*object.List)
	if !ok {
		return &object.Error{Message: "completion() first argument must be a list of messages"}
	}

	openaiMessages := make([]openai.Message, 0, len(messagesList.Elements))
	for i, msgObj := range messagesList.Elements {
		msgDict, ok := msgObj.(*object.Dict)
		if !ok {
			return &object.Error{Message: fmt.Sprintf("message %d must be a dict with 'role' and 'content' keys", i)}
		}

		role, content := "", ""
		for _, pair := range msgDict.Pairs {
			key := pair.Key.(*object.String).Value
			if key == "role" {
				if roleStr, ok := pair.Value.(*object.String); ok {
					role = roleStr.Value
				}
			} else if key == "content" {
				if contentStr, ok := pair.Value.(*object.String); ok {
					content = contentStr.Value
				}
			}
		}

		if role == "" || content == "" {
			return &object.Error{Message: fmt.Sprintf("message %d missing 'role' or 'content' key", i)}
		}

		openaiMessages = append(openaiMessages, openai.Message{
			Role:    role,
			Content: content,
		})
	}

	// Create independent context for AI completion to prevent script timeout from canceling AI operations
	aiCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Copy user from original context if it exists
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
	}

	// Create chat completion request
	req := openai.ChatCompletionRequest{
		Messages: openaiMessages,
	}

	// Call OpenAI with MCP server integration using the AI-specific context
	response, err := openaiClient.ChatCompletion(aiCtx, req)
	if err != nil {
		errMsg := fmt.Sprintf("AI completion failed: %v", err)
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
			errMsg += "\nNote: Make sure OpenAI API is properly configured"
		}
		return &object.Error{Message: errMsg}
	}

	if len(response.Choices) == 0 {
		return &object.String{Value: ""}
	}

	content := response.Choices[0].Message.GetContentAsString()
	return &object.String{Value: content}
}

// aiListTools fetches available tools via API for local/remote environments
func aiListTools(ctx context.Context, client *apiclient.ApiClient, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if client == nil {
		return &object.Error{Message: "AI tools not available - API client not configured"}
	}

	// Create independent context for tool listing to prevent script timeout
	aiCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Call API to get tools
	var tools []Tool
	_, err := client.Do(aiCtx, "GET", "api/chat/tools", nil, &tools)
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

// aiCallTool calls a tool via API for local/remote environments
func aiCallTool(ctx context.Context, client *apiclient.ApiClient, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "call_tool() requires tool name and arguments"}
	}

	if client == nil {
		return &object.Error{Message: "AI tools not available - API client not configured"}
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
	aiCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Call API to execute tool
	var response ToolCallResponse
	_, err := client.Do(aiCtx, "POST", "api/chat/tools/call", req, &response)
	if err != nil {
		errMsg := fmt.Sprintf("Tool call failed: %v", err)
		return &object.Error{Message: errMsg}
	}

	return convertToScriptlingObject(response.Content)
}

// aiListToolsMCP fetches available tools directly from MCP server
func aiListToolsMCP(ctx context.Context, openaiClient *openai.Client, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if openaiClient == nil {
		return &object.Error{Message: "AI tools not available - OpenAI client not configured"}
	}

	// Create independent context for tool listing
	aiCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Copy user from original context if it exists
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
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

// aiCallToolMCP calls a tool directly on MCP server
func aiCallToolMCP(ctx context.Context, openaiClient *openai.Client, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "call_tool() requires tool name and arguments"}
	}

	if openaiClient == nil {
		return &object.Error{Message: "AI tools not available - OpenAI client not configured"}
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
	aiCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Copy user from original context if it exists
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
	}

	// Call tool on MCP server
	response, err := openaiClient.GetMCPServer().CallTool(aiCtx, toolName, arguments)
	if err != nil {
		errMsg := fmt.Sprintf("Tool call failed: %v", err)
		return &object.Error{Message: errMsg}
	}

	return convertToScriptlingObject(response.Content)
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