package mcp

import (
	"fmt"

	"github.com/BurntSushi/toml"
	mcppkg "github.com/paularlott/mcp"
	"github.com/paularlott/mcp/toolmetadata"
)

// FromToml parses a TOML schema definition and returns MCP parameters
func FromToml(tomlStr string) ([]mcppkg.Parameter, error) {
	// Parse TOML into toolmetadata struct
	var metadata toolmetadata.ToolMetadata
	if _, err := toml.Decode(tomlStr, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	// Convert toolmetadata parameters to MCP parameters
	params := make([]mcppkg.Parameter, 0, len(metadata.Parameters))
	for _, param := range metadata.Parameters {
		var opts []mcppkg.Option
		if param.Required {
			opts = append(opts, mcppkg.Required())
		}

		var mcpParam mcppkg.Parameter
		switch param.Type {
		case "string":
			mcpParam = mcppkg.String(param.Name, param.Description, opts...)
		case "int", "integer":
			mcpParam = mcppkg.Number(param.Name, param.Description, opts...)
		case "float", "number":
			mcpParam = mcppkg.Number(param.Name, param.Description, opts...)
		case "bool", "boolean":
			mcpParam = mcppkg.Boolean(param.Name, param.Description, opts...)
		case "array:string":
			mcpParam = mcppkg.StringArray(param.Name, param.Description, opts...)
		case "array:number", "array:int", "array:integer", "array:float":
			mcpParam = mcppkg.NumberArray(param.Name, param.Description, opts...)
		case "array:bool", "array:boolean":
			mcpParam = mcppkg.BooleanArray(param.Name, param.Description, opts...)
		default:
			mcpParam = mcppkg.String(param.Name, param.Description, opts...)
		}

		params = append(params, mcpParam)
	}

	return params, nil
}
