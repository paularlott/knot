package mcptools

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/mcp"
	"github.com/paularlott/mcp/toolmetadata"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/object"
)

type Tool struct {
	Name     string
	Script   string
	Metadata *toolmetadata.ToolMetadata
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

		// Extract tool name from filename
		toolName := strings.TrimSuffix(filepath.Base(path), ".toml")

		// Check if disabled
		if disabled[toolName] {
			log.WithGroup("mcptools").Info("Skipping disabled tool", "name", toolName)
			skipped++
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

		// Load corresponding .py file
		scriptPath := strings.TrimSuffix(path, ".toml") + ".py"
		scriptData, err := fs.ReadFile(toolFS, scriptPath)
		if err != nil {
			log.WithGroup("mcptools").Error("Failed to read script", "path", scriptPath, "error", err)
			failed++
			return nil
		}

		// Register tool
		registry[toolName] = &Tool{
			Name:     toolName,
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

	// Convert params to scriptling objects for mcp library
	mcpParams := make(map[string]object.Object)
	for key, value := range params {
		mcpParams[key] = conversion.FromGo(value)
	}

	// Create a temporary script model for execution
	script := &model.Script{
		Content: tool.Script,
	}

	// Execute via scriptling (boot-loaded tools don't have AI client access)
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
		// Filter by visibility
		if visibility == "native" && tool.Metadata.Discoverable {
			continue
		}
		if visibility == "discoverable" && !tool.Metadata.Discoverable {
			continue
		}

		// Build tool using upstream builder
		toolBuilder := toolmetadata.BuildMCPTool(tool.Name, tool.Metadata)
		mcpTool := toolBuilder.ToMCPTool()
		tools = append(tools, mcpTool)
	}

	return tools
}

// GetAllMCPTools returns all MCP tools with their Visibility field set appropriately
func GetAllMCPTools() []mcp.MCPTool {
	mu.RLock()
	defer mu.RUnlock()

	tools := make([]mcp.MCPTool, 0, len(registry))
	for _, tool := range registry {
		toolBuilder := toolmetadata.BuildMCPTool(tool.Name, tool.Metadata)
		mcpTool := toolBuilder.ToMCPTool()
		tools = append(tools, mcpTool)
	}

	return tools
}
