package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/chat"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/mcp"
)

// scriptToolsProvider implements mcp.ToolProvider for native script tools
type scriptToolsProvider struct {
	user       *model.User
	onDemandOnly bool
}

// NewScriptToolsProvider creates a new script tools provider for a user
func NewScriptToolsProvider(user *model.User) *scriptToolsProvider {
	return &scriptToolsProvider{user: user, onDemandOnly: false}
}

// NewOnDemandScriptToolsProvider creates a new on-demand script tools provider for a user
func NewOnDemandScriptToolsProvider(user *model.User) *scriptToolsProvider {
	return &scriptToolsProvider{user: user, onDemandOnly: true}
}

// GetTools returns all available script tools for the user
// Implements user override behavior: user scripts override global scripts with the same name
func (p *scriptToolsProvider) GetTools(ctx context.Context) ([]mcp.MCPTool, error) {
	log.Debug("scriptToolsProvider.GetTools called", "user", p.user.Username, "user_id", p.user.Id)
	db := database.GetInstance()
	scripts, err := db.GetScripts()
	if err != nil {
		log.Warn("scriptToolsProvider.GetTools failed to get scripts", "error", err)
		return nil, err
	}
	log.Debug("scriptToolsProvider.GetTools fetched scripts", "count", len(scripts))

	// Use maps to ensure one tool per name (user scripts override global scripts)
	toolsMap := make(map[string]mcp.MCPTool)
	userIdMap := make(map[string]string) // Track which tools belong to which user

	for _, script := range scripts {
		// Skip non-tool scripts, inactive scripts, or deleted scripts
		if script.ScriptType != "tool" || !script.Active || script.IsDeleted {
			continue
		}

		// Check if script is valid for current zone
		if !service.CanUserExecuteScript(p.user, script) {
			continue
		}

		// Check zone restrictions
		currentZone := config.GetServerConfig().Zone
		if !script.IsValidForZone(currentZone) {
			log.Debug("scriptToolsProvider.GetTools: skipping tool due to zone", "tool", script.Name, "zones", script.Zones, "currentZone", currentZone)
			continue
		}

		// User scripts override global scripts with the same name
		if _, exists := userIdMap[script.Name]; exists {
			// If current script is global (UserId is empty)
			if script.UserId == "" {
				// Skip if we already have ANY tool (user or global) for this name
				// Global scripts should never replace existing tools
				continue
			}
			// Current script is a user script - always replace (user scripts override both global and other user scripts)
		}

		// Filter based on provider type
		if p.onDemandOnly {
			// On-demand provider: only return discoverable tools
			if !script.OnDemandTool {
				continue
			}
		} else {
			// Native provider: only return non-discoverable tools
			if script.OnDemandTool {
				continue
			}
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

		// Store tool and track ownership
		toolsMap[script.Name] = mcp.MCPTool{
			Name:        script.Name,
			Description: script.Description,
			InputSchema: inputSchema,
			Keywords:    script.MCPKeywords,
		}
		userIdMap[script.Name] = script.UserId // Empty string for global scripts
	}

	// Convert map to slice
	tools := make([]mcp.MCPTool, 0, len(toolsMap))
	for _, tool := range toolsMap {
		tools = append(tools, tool)
	}

	log.Debug("scriptToolsProvider.GetTools returning tools", "count", len(tools), "user", p.user.Username)
	for _, tool := range tools {
		log.Debug("scriptToolsProvider.GetTools tool", "name", tool.Name, "description", tool.Description, "keywords", tool.Keywords)
	}

	return tools, nil
}

// ExecuteTool executes a script tool by name
func (p *scriptToolsProvider) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	// Resolve script with user override and zone filtering
	script, err := service.ResolveScriptByName(name, p.user.Id)
	if err != nil || script.ScriptType != "tool" {
		return nil, nil // Tool not found - let other providers handle it
	}

	// Check permissions
	if !service.CanUserExecuteScript(p.user, script) {
		return nil, nil
	}

	// Check zone restrictions
	currentZone := config.GetServerConfig().Zone
	if !script.IsValidForZone(currentZone) {
		return nil, nil
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
	result, err := service.ExecuteScriptWithMCP(script, mcpParams, p.user)
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
