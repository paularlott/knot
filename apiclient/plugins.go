package apiclient

import "context"

// PluginPermissionInfo is one permission a loaded plugin declares, already
// qualified to its grant form (plugin.<name>.<id>).
type PluginPermissionInfo struct {
	Id    string `json:"id"`
	Label string `json:"label"`
}

type PluginPageInfo struct {
	Path       string `json:"path"`
	URL        string `json:"url"`
	Handler    string `json:"handler"`
	Label      string `json:"label,omitempty"`
	Permission string `json:"permission,omitempty"`
	Group      string `json:"group,omitempty"`
	MenuLabel  string `json:"menu_label,omitempty"`
}

type PluginMenuInfo struct {
	Label      string `json:"label"`
	URL        string `json:"url"`
	Permission string `json:"permission,omitempty"`
	Group      string `json:"group,omitempty"`
	Icon       string `json:"icon,omitempty"`
}

type PluginPeerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
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
	Pages       []PluginPageInfo       `json:"pages"`
	Peers       []PluginPeerInfo       `json:"peers,omitempty"`
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
