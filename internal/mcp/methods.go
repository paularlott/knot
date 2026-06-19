package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/methods"
	mcplib "github.com/paularlott/mcp"
)

type methodToolsProvider struct {
	user *model.User
}

func NewMethodToolsProvider(user *model.User) *methodToolsProvider {
	return &methodToolsProvider{user: user}
}

func (p *methodToolsProvider) GetTools(ctx context.Context) ([]mcplib.MCPTool, error) {
	infos := methods.DefaultRegistry().List(p.user)
	tools := []mcplib.MCPTool{}
	seen := map[string]bool{}
	for _, info := range infos {
		if !info.MCPTool {
			continue
		}
		toolName := methods.MCPToolName(info.Name)
		if seen[toolName] {
			continue
		}
		seen[toolName] = true
		schema := info.ParamsSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		tools = append(tools, mcplib.MCPTool{
			Name:        toolName,
			Description: info.Description,
			InputSchema: schema,
			Keywords:    info.Keywords,
			Visibility:  mcplib.ToolVisibilityDiscoverable,
		})
	}
	return tools, nil
}

func (p *methodToolsProvider) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	for _, info := range methods.DefaultRegistry().List(p.user) {
		if !info.MCPTool || methods.MCPToolName(info.Name) != name {
			continue
		}
		entry, localName, err := methods.DefaultRegistry().Pick(info.Name, p.user)
		if err != nil {
			return nil, err
		}
		defer methods.DefaultRegistry().Done(entry)

		session := agent_server.GetSession(entry.SpaceID)
		if session == nil {
			return nil, fmt.Errorf("no live method server is available")
		}
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		response, err := session.SendCallMethod(&msg.CallMethodRequest{
			Method: localName,
			Params: raw,
			ID:     1,
		}, entry.Server.Timeout)
		if err != nil {
			return nil, err
		}
		if response.Response.Error != nil {
			return nil, errors.New(response.Response.Error.Message)
		}
		return response.Response.Result, nil
	}
	return nil, nil
}
