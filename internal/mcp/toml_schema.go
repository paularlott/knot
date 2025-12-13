package mcp

import (
	"fmt"

	"github.com/BurntSushi/toml"
	mcppkg "github.com/paularlott/mcp"
)

// FromToml parses a TOML schema definition and returns MCP parameters
func FromToml(tomlStr string) ([]mcppkg.Parameter, error) {
	var schemaMap map[string]interface{}
	if _, err := toml.Decode(tomlStr, &schemaMap); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	var params []mcppkg.Parameter
	for name, value := range schemaMap {
		paramMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		paramType, _ := paramMap["type"].(string)
		description, _ := paramMap["description"].(string)
		required, _ := paramMap["required"].(bool)

		var param mcppkg.Parameter
		var opts []mcppkg.Option
		if required {
			opts = append(opts, mcppkg.Required())
		}

		switch paramType {
		case "string":
			param = mcppkg.String(name, description, opts...)
		case "number":
			param = mcppkg.Number(name, description, opts...)
		case "boolean":
			param = mcppkg.Boolean(name, description, opts...)
		case "array":
			param = mcppkg.StringArray(name, description, opts...)
		default:
			param = mcppkg.String(name, description, opts...)
		}

		params = append(params, param)
	}

	return params, nil
}
