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

// HandleGetPlugins returns the plugin inventory: loaded plugins with their
// declared permissions, menus and peers, plus failed plugins and load
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
				Group:      menu.Group,
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
				Group:      page.Group,
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
