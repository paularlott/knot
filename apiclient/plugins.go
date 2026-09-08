package apiclient

import "context"

// PluginPermissionInfo is one permission a loaded plugin declares, already
// qualified to its grant form (plugin.<name>.<id>).
type PluginPermissionInfo struct {
	Id     string `json:"id"`
	Label  string `json:"label"`
	Plugin string `json:"plugin,omitempty"` // set in the /api/permissions catalog
}

type PluginPageInfo struct {
	Path       string `json:"path"`
	URL        string `json:"url"`
	Handler    string `json:"handler"`
	Label      string `json:"label,omitempty"`
	Permission string `json:"permission,omitempty"`
	MenuLabel  string `json:"menu_label,omitempty"`
}

type PluginMenuInfo struct {
	Label      string `json:"label"`
	URL        string `json:"url"`
	Permission string `json:"permission,omitempty"`
	Icon       string `json:"icon,omitempty"`
}

type PluginPeerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

// PluginMCPToolInfo is one MCP tool a plugin exposes.
type PluginMCPToolInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Handler     string   `json:"handler"`
	Permission  string   `json:"permission,omitempty"`
	Parameters  []string `json:"parameters,omitempty"`
}

// PluginHandlerInfo is one declared ajax handler.
type PluginHandlerInfo struct {
	Handler    string   `json:"handler"`
	Permission string   `json:"permission,omitempty"`
}

// PluginFieldHandlerInfo is one declared field handler.
type PluginFieldHandlerInfo struct {
	Id         string   `json:"id"`
	Label      string   `json:"label"`
	Permission string   `json:"permission,omitempty"`
}

// PluginLibInfo is one in-process scriptling library.
type PluginLibInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type PluginInfo struct {
	Name        string                `json:"name"`
	Version     string                `json:"version"`
	Description string                `json:"description"`
	LogoLight   string                `json:"logo_light,omitempty"`
	LogoDark    string                `json:"logo_dark,omitempty"`
	SiteLogo    bool                  `json:"site_logo,omitempty"`
	Permissions []PluginPermissionInfo `json:"permissions"`
	Menus       []PluginMenuInfo       `json:"menus"`
	Pages         []PluginPageInfo         `json:"pages"`
	Peers         []PluginPeerInfo         `json:"peers,omitempty"`
	MCPTools      []PluginMCPToolInfo      `json:"mcp_tools,omitempty"`
	Handlers      []PluginHandlerInfo      `json:"handlers,omitempty"`
	FieldHandlers []PluginFieldHandlerInfo `json:"field_handlers,omitempty"`
	Libs          []PluginLibInfo          `json:"libs,omitempty"`
}

type FailedPluginInfo struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type PluginInfoList struct {
	Count    int                `json:"count"`
	Plugins  []PluginInfo       `json:"plugins"`
	Failed   []FailedPluginInfo `json:"failed"`
	Warnings []string           `json:"warnings"`
}

func (c *ApiClient) GetPlugins(ctx context.Context) (*PluginInfoList, int, error) {
	response := &PluginInfoList{}

	code, err := c.httpClient.Get(ctx, "/api/plugins", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
