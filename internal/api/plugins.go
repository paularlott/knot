package api

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
)

// AssetURL returns the served URL for a plugin asset, or "" when the plugin
// declares none.
func AssetURL(pluginName, rel string) string {
	if rel == "" {
		return ""
	}
	return "/plugins/" + pluginName + "/assets/" + rel
}

// HandleGetPlugins returns the plugin inventory: loaded plugins with
// everything they declare — permissions, menus, pages, MCP tools, handlers,
// field handlers, peers (Go and scriptling) — plus failed plugins and load
// warnings. This is the admin plugins page's data source and feeds the role
// editor's plugin-permissions section.
func HandleGetPlugins(w http.ResponseWriter, r *http.Request) {
	registry := plugins.GetRegistry()
	if registry == nil {
		rest.WriteResponse(http.StatusOK, w, r, apiclient.PluginInfoList{})
		return
	}

	list := apiclient.PluginInfoList{}
	for _, p := range registry.All() {
		info := apiclient.PluginInfo{
			Name:        p.Name,
			Version:     p.Version,
			Description: p.Description,
			LogoLight:   AssetURL(p.Name, p.LogoLight),
			LogoDark:    AssetURL(p.Name, p.LogoDark),
			SiteLogo:    p.SiteLogo,
			Permissions: make([]apiclient.PluginPermissionInfo, 0, len(p.Permissions)),
			Menus:       make([]apiclient.PluginMenuInfo, 0, len(p.Menus)),
			Pages:       make([]apiclient.PluginPageInfo, 0, len(p.Pages)),
		}
		for _, perm := range p.Permissions {
			info.Permissions = append(info.Permissions, apiclient.PluginPermissionInfo{Id: perm.Id, Label: perm.Label})
		}
		for _, menu := range p.Menus {
			info.Menus = append(info.Menus, apiclient.PluginMenuInfo{
				Label:      menu.Label,
				URL:        menu.URL,
				Permission: menu.Permission,
				Icon:       menu.Icon,
			})
		}
		for _, page := range p.Pages {
			info.Pages = append(info.Pages, apiclient.PluginPageInfo{
				Path:       page.Path,
				URL:        page.URL(),
				Handler:    page.Handler,
				Label:      page.Label,
				MenuLabel:  page.MenuLabel,
				Permission: page.Permission,
			})
		}
		for _, peer := range registry.Peers(p) {
			info.Peers = append(info.Peers, apiclient.PluginPeerInfo{
				Name:    peer.Name,
				Version: peer.Version,
				Healthy: peer.Healthy,
				Error:   peer.Error,
			})
		}
		for _, tool := range p.MCPTools {
			params := make([]string, 0, len(tool.Parameters))
			for _, param := range tool.Parameters {
				params = append(params, param.Name)
			}
			info.MCPTools = append(info.MCPTools, apiclient.PluginMCPToolInfo{
				Name:        tool.Name,
				Description: tool.Description,
				Handler:     tool.Handler,
				Permission:  tool.Permission,
				Parameters:  params,
			})
		}
		for _, decl := range p.Handlers {
			info.Handlers = append(info.Handlers, apiclient.PluginHandlerInfo{
				Handler:    decl.Handler,
				Permission: decl.Permission,
			})
		}
		for _, field := range p.FieldHandlers {
			info.FieldHandlers = append(info.FieldHandlers, apiclient.PluginFieldHandlerInfo{
				Id:         field.Id,
				Label:      field.Label,
				Permission: field.Permission,
			})
		}
		for _, peer := range p.ScriptPeers {
			info.ScriptPeers = append(info.ScriptPeers, apiclient.PluginScriptPeerInfo{
				Name:    peer.Name,
				Version: peer.Version,
			})
		}
		list.Plugins = append(list.Plugins, info)
	}
	list.Count = len(list.Plugins)

	for _, failed := range registry.Failed() {
		list.Failed = append(list.Failed, apiclient.FailedPluginInfo{
			Name:   failed.Name,
			Path:   failed.Path,
			Reason: failed.Reason,
		})
	}
	list.Warnings = registry.Warnings()

	rest.WriteResponse(http.StatusOK, w, r, list)
}
