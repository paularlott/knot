package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/chat"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/mcp"
)

// scriptToolsProvider implements mcp.ToolProvider for dynamic script tools
type scriptToolsProvider struct {
	user *model.User
}

// NewScriptToolsProvider creates a new script tools provider for a user
func NewScriptToolsProvider(user *model.User) *scriptToolsProvider {
	return &scriptToolsProvider{user: user}
}

// GetTools returns all available script tools for the user
func (p *scriptToolsProvider) GetTools(ctx context.Context) ([]mcp.MCPTool, error) {
	db := database.GetInstance()
	scripts, err := db.GetScripts()
	if err != nil {
		return nil, err
	}

	var tools []mcp.MCPTool
	for _, script := range scripts {
		// Skip non-tool scripts, inactive scripts, or deleted scripts
		if script.ScriptType != "tool" || !script.Active || script.IsDeleted {
			continue
		}

		// Check group access
		if len(script.Groups) > 0 && !p.user.HasAnyGroup(&script.Groups) {
			continue
		}

		// Build input schema - if TOML schema exists, parse it; otherwise use empty schema
		inputSchema := map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}

		if script.MCPInputSchemaToml != "" {
			// Parse TOML to get the parameters
			params, err := FromToml(script.MCPInputSchemaToml)
			if err != nil {
				log.Warn("Failed to parse TOML schema for script %s: %v", script.Name, err)
			} else if len(params) > 0 {
				// Build a temporary tool to extract the schema
				tempTool := mcp.NewTool(script.Name, script.Description, params...)
				inputSchema = tempTool.BuildSchema()
			}
		}

		// Build MCP tool
		tools = append(tools, mcp.MCPTool{
			Name:        script.Name,
			Description: script.Description,
			InputSchema: inputSchema,
			Keywords:    script.MCPKeywords,
		})
	}

	return tools, nil
}

// ExecuteTool executes a script tool by name
func (p *scriptToolsProvider) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	db := database.GetInstance()
	scripts, err := db.GetScripts()
	if err != nil {
		return nil, err
	}

	// Find the script
	var script *model.Script
	for _, s := range scripts {
		if s.Name == name && s.ScriptType == "tool" && s.Active && !s.IsDeleted {
			// Check group access
			if len(s.Groups) > 0 && !p.user.HasAnyGroup(&s.Groups) {
				continue
			}
			script = s
			break
		}
	}

	if script == nil {
		return nil, nil // Tool not found - let other providers handle it
	}

	// Convert params to string map for script execution
	mcpParams := make(map[string]string)
	for key, value := range params {
		switch v := value.(type) {
		case string:
			mcpParams[key] = v
		case int, int64, float64, bool:
			mcpParams[key] = fmt.Sprintf("%v", v)
		default:
			jsonBytes, _ := json.Marshal(v)
			mcpParams[key] = string(jsonBytes)
		}
	}

	// Execute the script
	result, err := service.ExecuteScriptWithMCP(script, mcpParams, p.user, nil)
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}

	// Check if the result contains an AI completion request
	if strings.HasPrefix(result, "__AI_COMPLETION_REQUEST__:") {
		// Extract the messages
		messagesJSON := strings.TrimPrefix(result, "__AI_COMPLETION_REQUEST__:")
		var messages []map[string]string
		if err := json.Unmarshal([]byte(messagesJSON), &messages); err == nil {
			// Try to get chat service and complete the request
			if chatService := getChatService(); chatService != nil {
				// Convert to chat message format
				chatMessages := make([]chat.ChatMessage, 0, len(messages))
				for _, msg := range messages {
					chatMessages = append(chatMessages, chat.ChatMessage{
						Role:      msg["role"],
						Content:   msg["content"],
						Timestamp: time.Now().Unix(),
					})
				}

				// Get completion
				response, err := chatService.ChatCompletion(ctx, chatMessages, p.user)
				if err != nil {
					return fmt.Sprintf("AI completion failed: %s", err.Error()), nil
				}
				return response.Content, nil
			}
		}
		return "AI completion not available in MCP environment", nil
	}

	return result, nil
}

var chatService *chat.Service

// SetChatService sets the global chat service for MCP tools
func SetChatService(cs *chat.Service) {
	chatService = cs
}

// getChatService returns the global chat service
func getChatService() *chat.Service {
	return chatService
}

// RegisterScriptTools registers all script-based tools for a user on a server
func RegisterScriptTools(server *mcp.Server, user *model.User) {
	db := database.GetInstance()

	// Get active scripts of type "tool"
	scripts, err := db.GetScripts()
	if err != nil {
		log.Warn("Failed to get scripts for MCP tools: %v", err)
		return
	}

	for _, script := range scripts {
		// Skip non-tool scripts, inactive scripts, or deleted scripts
		if script.ScriptType != "tool" || !script.Active || script.IsDeleted {
			continue
		}

		// Check group access
		if len(script.Groups) > 0 && !user.HasAnyGroup(&script.Groups) {
			continue
		}

		// Build tool with schema from TOML
		var tool *mcp.ToolBuilder
		if script.MCPInputSchemaToml != "" {
			params, err := FromToml(script.MCPInputSchemaToml)
			if err != nil {
				log.Warn("Failed to parse TOML schema for script %s: %v", script.Name, err)
				continue
			}
			tool = mcp.NewTool(script.Name, script.Description, params...)
		} else {
			tool = mcp.NewTool(script.Name, script.Description)
		}

		// Register searchable tool with keywords
		if len(script.MCPKeywords) > 0 {
			server.RegisterTool(tool, executeScriptTool(script), script.MCPKeywords...)
		} else {
			server.RegisterTool(tool, executeScriptTool(script))
		}
	}
}

// executeScriptTool creates a handler for executing a script
func executeScriptTool(script *model.Script) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
		user := ctx.Value("user").(*model.User)

		mcpParams := make(map[string]string)
		for key, value := range req.Args() {
			switch v := value.(type) {
			case string:
				mcpParams[key] = v
			case int, int64, float64, bool:
				mcpParams[key] = fmt.Sprintf("%v", v)
			default:
				jsonBytes, _ := json.Marshal(v)
				mcpParams[key] = string(jsonBytes)
			}
		}

		result, err := service.ExecuteScriptWithMCP(script, mcpParams, user, nil)
		if err != nil {
			return mcp.NewToolResponseText(fmt.Sprintf("Error: %s", err.Error())), nil
		}

		// Check if the result contains an AI completion request
		if strings.HasPrefix(result, "__AI_COMPLETION_REQUEST__:") {
			// Extract the messages
			messagesJSON := strings.TrimPrefix(result, "__AI_COMPLETION_REQUEST__:")
			var messages []map[string]string
			if err := json.Unmarshal([]byte(messagesJSON), &messages); err == nil {
				// Try to get chat service and complete the request
				if chatService := getChatService(); chatService != nil {
					// Convert to chat message format
					chatMessages := make([]chat.ChatMessage, 0, len(messages))
					for _, msg := range messages {
						chatMessages = append(chatMessages, chat.ChatMessage{
							Role:      msg["role"],
							Content:   msg["content"],
							Timestamp: time.Now().Unix(),
						})
					}

					// Get completion
					response, err := chatService.ChatCompletion(ctx, chatMessages, user)
					if err != nil {
						return mcp.NewToolResponseText(fmt.Sprintf("AI completion failed: %s", err.Error())), nil
					}
					return mcp.NewToolResponseText(response.Content), nil
				}
			}
			return mcp.NewToolResponseText("AI completion not available in MCP environment"), nil
		}

		return mcp.NewToolResponseText(result), nil
	}
}
