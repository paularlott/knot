package mcptools

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/mcp"
)

type Tool struct {
	Name       string
	Script     string
	Metadata   *ToolMetadata
}

var (
	registry = make(map[string]*Tool)
	mu       sync.RWMutex
)

func LoadTools(toolsPath string, disabledTools []string) error {
	mu.Lock()
	defer mu.Unlock()

	// Clear existing registry
	registry = make(map[string]*Tool)

	// Build disabled set
	disabled := make(map[string]bool)
	for _, name := range disabledTools {
		disabled[name] = true
	}

	var toolFS fs.FS
	var source string

	if toolsPath != "" {
		toolFS = os.DirFS(toolsPath)
		source = "filesystem: " + toolsPath
	} else {
		toolFS = GetEmbeddedFS()
		source = "embedded"
	}

	loaded := 0
	skipped := 0
	failed := 0

	// Scan for .toml files
	err := fs.WalkDir(toolFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".toml") {
			return nil
		}

		// Read TOML metadata
		tomlData, err := fs.ReadFile(toolFS, path)
		if err != nil {
			log.WithGroup("mcptools").Error("Failed to read TOML", "path", path, "error", err)
			failed++
			return nil
		}

		metadata, err := ParseMetadata(tomlData)
		if err != nil {
			log.WithGroup("mcptools").Error("Failed to parse metadata", "path", path, "error", err)
			failed++
			return nil
		}

		// Check if disabled
		if disabled[metadata.Name] {
			log.WithGroup("mcptools").Info("Skipping disabled tool", "name", metadata.Name)
			skipped++
			return nil
		}

		// Load corresponding .py file
		scriptPath := strings.TrimSuffix(path, ".toml") + ".py"
		scriptData, err := fs.ReadFile(toolFS, scriptPath)
		if err != nil {
			log.WithGroup("mcptools").Error("Failed to read script", "path", scriptPath, "error", err)
			failed++
			return nil
		}

		// Register tool
		registry[metadata.Name] = &Tool{
			Name:     metadata.Name,
			Script:   string(scriptData),
			Metadata: metadata,
		}
		loaded++

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to scan tools: %w", err)
	}

	log.WithGroup("mcptools").Info("Loaded MCP tools", "source", source, "loaded", loaded, "skipped", skipped, "failed", failed)
	return nil
}

func ReloadTools(toolsPath string, disabledTools []string) (int, error) {
	if err := LoadTools(toolsPath, disabledTools); err != nil {
		return 0, err
	}
	mu.RLock()
	defer mu.RUnlock()
	return len(registry), nil
}

func GetTool(name string) (*Tool, bool) {
	mu.RLock()
	defer mu.RUnlock()
	tool, ok := registry[name]
	return tool, ok
}

func ListTools() []*Tool {
	mu.RLock()
	defer mu.RUnlock()
	tools := make([]*Tool, 0, len(registry))
	for _, tool := range registry {
		tools = append(tools, tool)
	}
	return tools
}

func ExecuteTool(name string, params map[string]interface{}, user *model.User) (interface{}, error) {
	tool, ok := GetTool(name)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	// Convert params to string map for scriptling
	mcpParams := make(map[string]string)
	for key, value := range params {
		switch v := value.(type) {
		case string:
			mcpParams[key] = v
		case []interface{}:
			// Convert array to comma-separated string for mcpGetList
			strs := make([]string, len(v))
			for i, elem := range v {
				if strVal, ok := elem.(string); ok {
					strs[i] = strVal
				} else {
					strs[i] = fmt.Sprintf("%v", elem)
				}
			}
			mcpParams[key] = strings.Join(strs, ",")
		default:
			mcpParams[key] = fmt.Sprintf("%v", v)
		}
	}

	// Create a temporary script model for execution
	script := &model.Script{
		Content: tool.Script,
	}

	// Execute via scriptling
	result, err := service.ExecuteScriptWithMCP(script, mcpParams, user)
	if err != nil {
		// Strip MCP_TOOL_ERROR prefix if present
		if strings.HasPrefix(err.Error(), "MCP_TOOL_ERROR: ") {
			return nil, fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "MCP_TOOL_ERROR: "))
		}
		return nil, err
	}

	return result, nil
}

func GetMCPTools(visibility string) []mcp.MCPTool {
	mu.RLock()
	defer mu.RUnlock()

	tools := make([]mcp.MCPTool, 0)
	for _, tool := range registry {
		if tool.Metadata.Visibility != visibility {
			continue
		}

		// Build input schema from metadata
		inputSchema := map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}

		if len(tool.Metadata.Parameters) > 0 {
			properties := make(map[string]interface{})
			required := make([]string, 0)

			for paramName, param := range tool.Metadata.Parameters {
				properties[paramName] = map[string]interface{}{
					"type":        param.Type,
					"description": param.Description,
				}
				if param.Required {
					required = append(required, paramName)
				}
			}

			inputSchema["properties"] = properties
			if len(required) > 0 {
				inputSchema["required"] = required
			}
		}

		// Determine MCP visibility
		mcpVisibility := mcp.ToolVisibilityNative
		if tool.Metadata.Visibility == "discoverable" {
			mcpVisibility = mcp.ToolVisibilityDiscoverable
		}

		tools = append(tools, mcp.MCPTool{
			Name:        tool.Metadata.Name,
			Description: tool.Metadata.Description,
			InputSchema: inputSchema,
			Keywords:    tool.Metadata.Keywords,
			Visibility:  mcpVisibility,
		})
	}

	return tools
}

// GetAllMCPTools returns all MCP tools with their Visibility field set appropriately
func GetAllMCPTools() []mcp.MCPTool {
	mu.RLock()
	defer mu.RUnlock()

	tools := make([]mcp.MCPTool, 0, len(registry))
	for _, tool := range registry {
		// Build input schema from metadata
		inputSchema := map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}

		if len(tool.Metadata.Parameters) > 0 {
			properties := make(map[string]interface{})
			required := make([]string, 0)

			for paramName, param := range tool.Metadata.Parameters {
				properties[paramName] = map[string]interface{}{
					"type":        param.Type,
					"description": param.Description,
				}
				if param.Required {
					required = append(required, paramName)
				}
			}

			inputSchema["properties"] = properties
			if len(required) > 0 {
				inputSchema["required"] = required
			}
		}

		// Determine MCP visibility
		mcpVisibility := mcp.ToolVisibilityNative
		if tool.Metadata.Visibility == "discoverable" {
			mcpVisibility = mcp.ToolVisibilityDiscoverable
		}

		tools = append(tools, mcp.MCPTool{
			Name:        tool.Metadata.Name,
			Description: tool.Metadata.Description,
			InputSchema: inputSchema,
			Keywords:    tool.Metadata.Keywords,
			Visibility:  mcpVisibility,
		})
	}

	return tools
}
