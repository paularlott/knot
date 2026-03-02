package mcptools

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/paularlott/mcp/toolmetadata"
)

func ParseMetadata(tomlContent []byte) (*toolmetadata.ToolMetadata, error) {
	var metadata toolmetadata.ToolMetadata
	if err := toml.Unmarshal(tomlContent, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	if metadata.Description == "" {
		return nil, fmt.Errorf("tool description is required")
	}

	return &metadata, nil
}
