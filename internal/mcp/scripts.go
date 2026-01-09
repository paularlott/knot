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
	"github.com/paularlott/mcp/discovery"
)

var chatService *chat.Service

// SetChatService sets the global chat service for MCP tools
func SetChatService(cs *chat.Service) {
	chatService = cs
}

// getChatService returns the global chat service
func getChatService() *chat.Service {
	return chatService
}

// RegisterScriptTools registers all script-based tools for a user
func RegisterScriptTools(registry *discovery.ToolRegistry, user *model.User) {
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

		// Register with keywords
		registry.RegisterTool(
			tool,
			executeScriptTool(script),
			script.MCPKeywords...,
		)
	}
}

// RegisterScriptToolsNative registers all script-based tools natively on a server for a user
func RegisterScriptToolsNative(server *mcp.Server, user *model.User) {
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

		// Register natively on the server
		server.RegisterTool(tool, executeScriptTool(script))
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
