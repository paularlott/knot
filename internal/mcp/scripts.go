package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/mcp"
	"github.com/paularlott/mcp/discovery"
)

// registerScriptTools registers all script-based tools for a user
func registerScriptTools(registry *discovery.ToolRegistry, user *model.User) {
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

// executeScriptTool creates a handler for executing a script
func executeScriptTool(script *model.Script) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
		// Extract parameters
		mcpParams := make(map[string]string)

		// Get all arguments
		for key, value := range req.Args() {
			// Convert complex types to JSON
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

		// Execute script locally
		result, err := executeScriptWithEnv(script, mcpParams)
		if err != nil {
			return mcp.NewToolResponseText(fmt.Sprintf("Error: %s", err.Error())), nil
		}

		return mcp.NewToolResponseText(result), nil
	}
}

// executeScriptWithEnv executes a script with MCP parameters
func executeScriptWithEnv(script *model.Script, mcpParams map[string]string) (string, error) {
	db := database.GetInstance()

	// Get all library scripts
	libraries := make(map[string]string)
	allScripts, err := db.GetScripts()
	if err == nil {
		for _, lib := range allScripts {
			if lib.IsDeleted || !lib.Active || lib.ScriptType != "lib" {
				continue
			}
			libraries[lib.Name] = lib.Content
		}
	}

	return service.ExecuteScriptWithMCP(script, libraries, mcpParams)
}
