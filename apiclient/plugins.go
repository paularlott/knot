package apiclient

import "context"

// PluginPermissionInfo is one permission a loaded plugin declares, already
// qualified to its grant form (plugin.<name>.<id>).
type PluginPermissionInfo struct {
	Id     string `json:"id" msgpack:"id"`
	Label  string `json:"label" msgpack:"label"`
	Plugin string `json:"plugin,omitempty" msgpack:"plugin,omitempty"` // set in the /api/permissions catalog
}

type PluginPageInfo struct {
	Path       string `json:"path" msgpack:"path"`
	URL        string `json:"url" msgpack:"url"`
	Handler    string `json:"handler" msgpack:"handler"`
	Label      string `json:"label,omitempty" msgpack:"label,omitempty"`
	Permission string `json:"permission,omitempty" msgpack:"permission,omitempty"`
	MenuLabel  string `json:"menu_label,omitempty" msgpack:"menu_label,omitempty"`
}

type PluginMenuInfo struct {
	Label      string `json:"label" msgpack:"label"`
	URL        string `json:"url" msgpack:"url"`
	Permission string `json:"permission,omitempty" msgpack:"permission,omitempty"`
	Icon       string `json:"icon,omitempty" msgpack:"icon,omitempty"`
}

type PluginPeerInfo struct {
	Name    string `json:"name" msgpack:"name"`
	Version string `json:"version" msgpack:"version"`
	Healthy bool   `json:"healthy" msgpack:"healthy"`
	Error   string `json:"error,omitempty" msgpack:"error,omitempty"`
}

// PluginMCPToolInfo is one MCP tool a plugin exposes.
type PluginMCPToolInfo struct {
	Name        string   `json:"name" msgpack:"name"`
	Description string   `json:"description" msgpack:"description"`
	Handler     string   `json:"handler" msgpack:"handler"`
	Permission  string   `json:"permission,omitempty" msgpack:"permission,omitempty"`
	Parameters  []string `json:"parameters,omitempty" msgpack:"parameters,omitempty"`
}

// PluginHandlerInfo is one declared ajax handler.
type PluginHandlerInfo struct {
	Handler    string `json:"handler" msgpack:"handler"`
	Permission string `json:"permission,omitempty" msgpack:"permission,omitempty"`
}

// PluginFieldHandlerInfo is one declared field handler.
type PluginFieldHandlerInfo struct {
	Id         string `json:"id" msgpack:"id"`
	Label      string `json:"label" msgpack:"label"`
	Permission string `json:"permission,omitempty" msgpack:"permission,omitempty"`
}

// PluginLibInfo is one in-process scriptling library.
type PluginLibInfo struct {
	Name    string `json:"name" msgpack:"name"`
	Version string `json:"version" msgpack:"version"`
}

type PluginInfo struct {
	Name          string                   `json:"name" msgpack:"name"`
	Version       string                   `json:"version" msgpack:"version"`
	Description   string                   `json:"description" msgpack:"description"`
	LogoLight     string                   `json:"logo_light,omitempty" msgpack:"logo_light,omitempty"`
	LogoDark      string                   `json:"logo_dark,omitempty" msgpack:"logo_dark,omitempty"`
	SiteLogo      bool                     `json:"site_logo,omitempty" msgpack:"site_logo,omitempty"`
	Permissions   []PluginPermissionInfo   `json:"permissions" msgpack:"permissions"`
	Menus         []PluginMenuInfo         `json:"menus" msgpack:"menus"`
	Pages         []PluginPageInfo         `json:"pages" msgpack:"pages"`
	Peers         []PluginPeerInfo         `json:"peers,omitempty" msgpack:"peers,omitempty"`
	MCPTools      []PluginMCPToolInfo      `json:"mcp_tools,omitempty" msgpack:"mcp_tools,omitempty"`
	Handlers      []PluginHandlerInfo      `json:"handlers,omitempty" msgpack:"handlers,omitempty"`
	FieldHandlers []PluginFieldHandlerInfo `json:"field_handlers,omitempty" msgpack:"field_handlers,omitempty"`
	Libs          []PluginLibInfo          `json:"libs,omitempty" msgpack:"libs,omitempty"`
}

type FailedPluginInfo struct {
	Name   string `json:"name" msgpack:"name"`
	Path   string `json:"path" msgpack:"path"`
	Reason string `json:"reason" msgpack:"reason"`
}

type PluginInfoList struct {
	Count    int                `json:"count" msgpack:"count"`
	Plugins  []PluginInfo       `json:"plugins" msgpack:"plugins"`
	Failed   []FailedPluginInfo `json:"failed" msgpack:"failed"`
	Warnings []string           `json:"warnings" msgpack:"warnings"`
}

func (c *ApiClient) GetPlugins(ctx context.Context) (*PluginInfoList, int, error) {
	response := &PluginInfoList{}

	code, err := c.httpClient.Get(ctx, "/api/plugins", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
