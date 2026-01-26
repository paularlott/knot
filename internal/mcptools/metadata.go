package mcptools

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type ToolMetadata struct {
	Name        string                       `toml:"name"`
	Description string                       `toml:"description"`
	Keywords    []string                     `toml:"keywords"`
	Visibility  string                       `toml:"visibility"` // "native" or "ondemand"
	Parameters  map[string]ParameterMetadata `toml:"parameters"`
	Output      *OutputMetadata              `toml:"output"`
}

type ParameterMetadata struct {
	Type        string `toml:"type"`
	Description string `toml:"description"`
	Required    bool   `toml:"required"`
}

type OutputMetadata struct {
	Type        string                   `toml:"type"`
	Description string                   `toml:"description"`
	Fields      map[string]FieldMetadata `toml:"fields"`
}

type FieldMetadata struct {
	Type        string                 `toml:"type"`
	Description string                 `toml:"description"`
	Items       map[string]interface{} `toml:"items"`
}

func ParseMetadata(tomlContent []byte) (*ToolMetadata, error) {
	var metadata ToolMetadata
	if err := toml.Unmarshal(tomlContent, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	// Validate required fields
	if metadata.Name == "" {
		return nil, fmt.Errorf("tool name is required")
	}
	if metadata.Description == "" {
		return nil, fmt.Errorf("tool description is required")
	}
	if metadata.Visibility == "" {
		metadata.Visibility = "native"
	}
	if metadata.Visibility != "native" && metadata.Visibility != "ondemand" {
		return nil, fmt.Errorf("visibility must be 'native' or 'ondemand'")
	}

	return &metadata, nil
}
