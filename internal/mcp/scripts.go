package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/mcptools"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/mcp"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/object"
)

// scriptToolsProvider implements mcp.ToolProvider for script tools
// Returns all tools with their Visibility field set appropriately
type scriptToolsProvider struct {
	user *model.User
}

// NewScriptToolsProvider creates a new script tools provider for a user
// Returns all tools (native and discoverable) with their visibility set
func NewScriptToolsProvider(user *model.User) *scriptToolsProvider {
	return &scriptToolsProvider{user: user}
}

// GetTools returns all available script tools for the user
// Each tool has its Visibility field set based on the script's Discoverable flag
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

		// Determine visibility based on script's Discoverable flag
		visibility := mcp.ToolVisibilityNative
		if script.Discoverable {
			visibility = mcp.ToolVisibilityDiscoverable
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

		// Store tool with visibility and track ownership
		toolsMap[script.Name] = mcp.MCPTool{
			Name:        script.Name,
			Description: script.Description,
			InputSchema: inputSchema,
			Keywords:    script.MCPKeywords,
			Visibility:  visibility,
		}
		userIdMap[script.Name] = script.UserId // Empty string for global scripts
	}

	// Plugin-exposed tools ([[tool.knot.mcp_tools]]): appended after
	// scripts, so a script with the same name shadows the plugin tool —
	// matching ExecuteTool's resolution order (boot, scripts, plugins).
	// Gates (permission/group) apply at listing exactly as at execution.
	if registry := plugins.GetRegistry(); registry != nil {
		for _, plugin := range registry.All() {
			for _, tool := range plugin.MCPTools {
				if _, taken := toolsMap[tool.Name]; taken {
					continue
				}
				if !pluginToolAllowed(p.user, &tool) {
					continue
				}
				toolsMap[tool.Name] = mcp.MCPTool{
					Name:        tool.Name,
					Description: tool.Description,
					InputSchema: pluginToolInputSchema(&tool),
					Visibility:  mcp.ToolVisibilityNative,
				}
			}
		}
	}

	// Convert map to slice
	tools := make([]mcp.MCPTool, 0, len(toolsMap))
	for _, tool := range toolsMap {
		tools = append(tools, tool)
	}

	// Add boot-loaded MCP tools (they set their own visibility)
	bootTools := mcptools.GetAllMCPTools()
	tools = append(tools, bootTools...)

	log.Debug("scriptToolsProvider.GetTools returning tools", "count", len(tools), "user", p.user.Username)
	for _, tool := range tools {
		log.Debug("scriptToolsProvider.GetTools tool", "name", tool.Name, "description", tool.Description, "keywords", tool.Keywords, "visibility", tool.Visibility)
	}

	return tools, nil
}

// ExecuteTool executes a script tool by name
func (p *scriptToolsProvider) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (*mcp.ToolResponse, error) {
	// Try boot-loaded tools first
	toolResult, toolErr := mcptools.ExecuteTool(name, params, p.user)
	if toolErr == nil {
		// Boot-loaded tool executed successfully
		return mcp.NewToolResponseAuto(toolResult), nil
	}
	// Check if tool exists in mcptools (error is "tool not found") vs execution failed
	if _, exists := mcptools.GetTool(name); exists {
		// Tool exists in mcptools but execution failed - return the error
		log.WithError(toolErr).Error("scriptToolsProvider.ExecuteTool: boot-loaded tool execution failed", "tool", name)
		return nil, toolErr
	}
	// Tool not in mcptools, try database scripts

	// Resolve script with user override and zone filtering
	script, err := service.ResolveScriptByName(name, p.user.Id)
	if err != nil {
		// Not a script — try plugin-exposed tools before giving up.
		result, perr, handled := p.executePluginTool(ctx, name, params)
		if handled {
			return result, perr
		}
		return nil, nil // Tool not found - let other providers handle it
	}
	if script.ScriptType != "tool" {
		return nil, fmt.Errorf("script '%s' is not a tool", name)
	}

	// Check permissions
	if !service.CanUserExecuteScript(p.user, script) {
		return nil, fmt.Errorf("permission denied to execute tool '%s'", name)
	}

	// Check zone restrictions
	currentZone := config.GetServerConfig().Zone
	if !script.IsValidForZone(currentZone) {
		return nil, fmt.Errorf("tool '%s' is not available in zone '%s'", name, currentZone)
	}

	// User-defined script — approval handled by lmchatkit UI, not SSE

	// Convert params to scriptling objects for script execution
	mcpParams := make(map[string]object.Object)
	for key, value := range params {
		mcpParams[key] = conversion.FromGo(value)
	}

	// Execute the script
	result, err := service.ExecuteScriptWithMCP(script, mcpParams, p.user)
	if err != nil {
		// Strip MCP_TOOL_ERROR prefix if present
		if strings.HasPrefix(err.Error(), "MCP_TOOL_ERROR: ") {
			return nil, fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "MCP_TOOL_ERROR: "))
		}
		return nil, err
	}

	return mcp.NewToolResponseAuto(result), nil
}

// pluginToolAllowed reports whether the user passes the tool's declared
// permission/group gate.
func pluginToolAllowed(user *model.User, tool *plugins.MCPTool) bool {
	if tool.Permission != "" && !user.HasPluginPermission(tool.Permission) {
		return false
	}
	if len(tool.Groups) > 0 && !user.HasAnyGroup(&tool.Groups) {
		return false
	}
	return true
}

// executePluginTool dispatches a plugin-exposed MCP tool by name. handled
// reports whether the name matched a plugin tool (result/err carry the
// outcome); when false the caller continues its not-found path.
func (p *scriptToolsProvider) executePluginTool(ctx context.Context, name string, params map[string]interface{}) (*mcp.ToolResponse, error, bool) {
	registry := plugins.GetRegistry()
	if registry == nil {
		return nil, nil, false
	}
	for _, plugin := range registry.All() {
		for i := range plugin.MCPTools {
			tool := &plugin.MCPTools[i]
			if tool.Name != name {
				continue
			}
			if !pluginToolAllowed(p.user, tool) {
				return nil, fmt.Errorf("permission denied to execute tool '%s'", name), true
			}
			result, response, exitCode, err := service.DispatchPluginMCPTool(ctx, apiclient.NewMuxClient(p.user), p.user, plugin, tool.Handler, params)
			switch {
			case exitCode != 0:
				// return_error: the response carries the actual error body —
				// prefer it over the bare SystemExit error, like script tools.
				if response != "" {
					return nil, fmt.Errorf("%s", response), true
				}
				if err != nil {
					return nil, err, true
				}
				return nil, fmt.Errorf("tool exited with code %d", exitCode), true
			case response != "":
				// return_string / return_object: the explicit response wins.
				return mcp.NewToolResponseAuto(response), nil, true
			default:
				// A plain return value: converted the handler way.
				if err != nil {
					return nil, err, true
				}
				return mcp.NewToolResponseAuto(result), nil, true
			}
		}
	}
	return nil, nil, false
}

// pluginToolInputSchema builds the MCP input schema from the tool's
// declared parameters — types map to JSON schema types, descriptions ride
// along for MCP clients and LLMs, defaults and required-ness included. An
// undeclared tool gets the permissive empty-object schema.
func pluginToolInputSchema(tool *plugins.MCPTool) map[string]interface{} {
	properties := map[string]interface{}{}
	required := []string{}
	for _, param := range tool.Parameters {
		prop := map[string]interface{}{"type": plugins.MCPToolParamSchemaType(param.Type)}
		if param.Description != "" {
			prop["description"] = param.Description
		}
		if param.Default != nil {
			prop["default"] = param.Default
		}
		properties[param.Name] = prop
		if param.Required {
			required = append(required, param.Name)
		}
	}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
