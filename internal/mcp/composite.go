package mcp

import (
	"context"

	mcplib "github.com/paularlott/mcp"
)

type compositeProvider struct {
	providers []mcplib.ToolProvider
}

func NewCompositeProvider(providers ...mcplib.ToolProvider) mcplib.ToolProvider {
	filtered := []mcplib.ToolProvider{}
	for _, provider := range providers {
		if provider != nil {
			filtered = append(filtered, provider)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return &compositeProvider{providers: filtered}
}

func (p *compositeProvider) GetTools(ctx context.Context) ([]mcplib.MCPTool, error) {
	tools := []mcplib.MCPTool{}
	for _, provider := range p.providers {
		providerTools, err := provider.GetTools(ctx)
		if err != nil {
			return nil, err
		}
		tools = append(tools, providerTools...)
	}
	return tools, nil
}

func (p *compositeProvider) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	for _, provider := range p.providers {
		result, err := provider.ExecuteTool(ctx, name, params)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return result, nil
		}
	}
	return nil, nil
}
