package web

import (
	"github.com/paularlott/knot/internal/filestore"
	"html/template"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
)

// NavItem is a single sidebar entry rendered from data in menus.tmpl. Icon
// holds the inner SVG markup (one or more <path/> elements) and is typed
// template.HTML so html/template renders it verbatim — plugin logo items use
// the same field to carry themed <img> markup. External marks an off-site
// URL, rendered with target="_blank" and an indicator.
type NavItem struct {
	URL      string
	Label    string
	Icon     template.HTML
	Starred  bool
	External bool
	Active   bool // owns the current page
}

// navIcon is the shared <svg> wrapper attributes applied to every nav entry's
// icon. The per-item Icon value supplies the inner path(s).
const navIconAttr = `class="nav-item-icon group shrink-0" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" aria-hidden="true"`

// icon returns a NavItem's complete <svg> element.
func navIcon(inner string) template.HTML {
	return template.HTML(`<svg ` + navIconAttr + `>` + inner + `</svg>`)
}

// Icon path data, lifted verbatim from the previous menus.tmpl so the sidebar
// looks identical. Each constant is the inner markup of the icon's <svg>.
const (
	iconSpaces    = `<path stroke-linecap="round" stroke-linejoin="round" d="M2.25 15a4.5 4.5 0 0 0 4.5 4.5H18a3.75 3.75 0 0 0 1.332-7.257 3 3 0 0 0-3.758-3.848 5.25 5.25 0 0 0-10.233 2.33A4.502 4.502 0 0 0 2.25 15Z" />`
	iconTunnels   = `<path stroke-linecap="round" stroke-linejoin="round" d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" />`
	iconTokens    = `<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 5.25a3 3 0 0 1 3 3m3 0a6 6 0 0 1-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1 1 21.75 8.25Z" />`
	iconVolumes   = `<path stroke-linecap="round" stroke-linejoin="round" d="M20.25 6.375c0 2.278-3.694 4.125-8.25 4.125S3.75 8.653 3.75 6.375m16.5 0c0-2.278-3.694-4.125-8.25-4.125S3.75 4.097 3.75 6.375m16.5 0v11.25c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125V6.375m16.5 0v3.75m-16.5-3.75v3.75m16.5 0v3.75C20.25 16.153 16.556 18 12 18s-8.25-1.847-8.25-4.125v-3.75m16.5 0c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125" />`
	iconTemplates = `<path stroke-linecap="round" stroke-linejoin="round" d="M14.25 9.75 16.5 12l-2.25 2.25m-4.5 0L7.5 12l2.25-2.25M6 20.25h12A2.25 2.25 0 0 0 20.25 18V6A2.25 2.25 0 0 0 18 3.75H6A2.25 2.25 0 0 0 3.75 6v12A2.25 2.25 0 0 0 6 20.25Z" />`
	iconVariables = `<path stroke-linecap="round" stroke-linejoin="round" d="M4.745 3A23.933 23.933 0 0 0 3 12c0 3.183.62 6.22 1.745 9M19.5 3c.967 2.78 1.5 5.817 1.5 9s-.533 6.22-1.5 9M8.25 8.885l1.444-.89a.75.75 0 0 1 1.105.402l2.402 7.206a.75.75 0 0 0 1.104.401l1.445-.889m-8.25.75.213.09a1.687 1.687 0 0 0 2.062-.617l4.45-6.676a1.688 1.688 0 0 1 2.062-.618l.213.09" />`
	iconStacks    = `<path stroke-linecap="round" stroke-linejoin="round" d="M6.429 9.75 2.25 12l4.179 2.25m0-4.5 5.571 3 5.571-3m-11.142 0L2.25 7.5 12 2.25l9.75 5.25-4.179 2.25m0 0L21.75 12l-4.179 2.25m0 0 4.179 2.25L12 21.75 2.25 16.5l4.179-2.25m11.142 0-5.571 3-5.571-3" />`
	iconScripts   = `<path stroke-linecap="round" stroke-linejoin="round" d="M6.75 7.5l3 2.25-3 2.25m4.5 0h3m-9 8.25h13.5A2.25 2.25 0 0021 18V6a2.25 2.25 0 00-2.25-2.25H5.25A2.25 2.25 0 003 6v12a2.25 2.25 0 002.25 2.25z" />`
	iconEvents    = `<path stroke-linecap="round" stroke-linejoin="round" d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0" />`
	iconSkills    = `<path stroke-linecap="round" stroke-linejoin="round" d="M12 6.042A8.967 8.967 0 1 0 6 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 0 1 6 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 0 1 6-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0 0 18 18a8.967 8.967 0 0 0-6 2.292m0-14.25v14.25" />`
	iconCommands  = `<path stroke-linecap="round" stroke-linejoin="round" d="M6.75 7.5l3 2.25-3 2.25m4.5 0h3m-9 8.25h13.5A2.25 2.25 0 0021 18V6a2.25 2.25 0 00-2.25-2.25H5.25A2.25 2.25 0 003 6v12a2.25 2.25 0 002.25 2.25z" />`
	iconMCP       = `<path stroke-linecap="round" stroke-linejoin="round" d="M21.75 17.25v-.228a4.5 4.5 0 0 0-.12-1.03l-2.268-9.64a3.375 3.375 0 0 0-3.285-2.602H7.923a3.375 3.375 0 0 0-3.285 2.602l-2.268 9.64a4.5 4.5 0 0 0-.12 1.03v.228m19.5 0a3 3 0 0 1-3 3H5.25a3 3 0 0 1-3-3m19.5 0a3 3 0 0 0-3-3H5.25a3 3 0 0 0-3 3m16.5 0h.008v.008h-.008v-.008Zm-3 0h.008v.008h-.008v-.008Zm-3 0h.008v.008h-.008v-.008Zm-3 0h.008v.008h-.008v-.008Zm-3 0h.008v.008h-.008v-.008Z" />`
	iconUsers     = `<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 6a3.75 3.75 0 1 1-7.5 0 3.75 3.75 0 0 1 7.5 0ZM4.501 20.118a7.5 7.5 0 0 1 14.998 0A17.933 17.933 0 0 1 12 21.75c-2.676 0-5.216-.584-7.499-1.632Z" />`
	iconGroups    = `<path stroke-linecap="round" stroke-linejoin="round" d="M15 19.128a9.38 9.38 0 0 0 2.625.372 9.337 9.337 0 0 0 4.121-.952 4.125 4.125 0 0 0-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 0 1 8.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0 1 11.964-3.07M12 6.375a3.375 3.375 0 1 1-6.75 0 3.375 3.375 0 0 1 6.75 0Zm8.25 2.25a2.625 2.625 0 1 1-5.25 0 2.625 2.625 0 0 1 5.25 0Z" />`
	iconRoles     = `<path stroke-linecap="round" stroke-linejoin="round" d="M18 18.72a9.094 9.094 0 0 0 3.741-.479 3 3 0 0 0-4.682-2.72m.94 3.198.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0 1 12 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 0 1 6 18.719m12 0a5.971 5.971 0 0 0-.941-3.197m0 0A5.995 5.995 0 0 0 12 12.75a5.995 5.995 0 0 0-5.058 2.772m0 0a3 3 0 0 0-4.681 2.72 8.986 8.986 0 0 0 3.74.477m.94-3.197a5.971 5.971 0 0 0-.94 3.197M15 6.75a3 3 0 1 1-6 0 3 3 0 0 1 6 0Zm6 3a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Zm-13.5 0a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Z" />`
	iconAudit     = `<path stroke-linecap="round" stroke-linejoin="round" d="M8.25 6.75h12M8.25 12h12m-12 5.25h12M3.75 6.75h.007v.008H3.75V6.75Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0ZM3.75 12h.007v.008H3.75V12Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Zm-.375 5.25h.007v.008H3.75v-.008Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Z" />`
	iconFiles     = `<path stroke-linecap="round" stroke-linejoin="round" d="M2.25 12.75V12A2.25 2.25 0 0 1 4.5 9.75h15A2.25 2.25 0 0 1 21.75 12v.75m-8.69-6.44-2.12-2.12a1.5 1.5 0 0 0-1.061-.44H4.5A2.25 2.25 0 0 0 2.25 6v12a2.25 2.25 0 0 0 2.25 2.25h15A2.25 2.25 0 0 0 21.75 18V9a2.25 2.25 0 0 0-2.25-2.25h-5.379a1.5 1.5 0 0 1-1.06-.44Z" />`
	iconCluster   = `<path stroke-linecap="round" stroke-linejoin="round" d="M5.25 14.25h13.5m-13.5 0a3 3 0 0 1-3-3m3 3a3 3 0 1 0 0 6h13.5a3 3 0 1 0 0-6m-16.5-3a3 3 0 0 1 3-3h13.5a3 3 0 0 1 3 3m-19.5 0a4.5 4.5 0 0 1 .9-2.7L5.737 5.1a3.375 3.375 0 0 1 2.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 0 1 .9 2.7m0 0a3 3 0 0 1-3 3m0 3h.008v.008h-.008v-.008Zm0-6h.008v.008h-.008v-.008Zm-3 6h.008v.008h-.008v-.008Zm0-6h.008v.008h-.008v-.008Z" />`
)

func nav(url, label, iconInner string) NavItem {
	return NavItem{URL: url, Label: label, Icon: navIcon(iconInner)}
}

// iconPlugin is the fallback icon for plugin menu items without a declared
// icon asset.
const iconPlugin = `<path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6A2.25 2.25 0 0 1 6 3.75h2.25A2.25 2.25 0 0 1 10.5 6v2.25a2.25 2.25 0 0 1-2.25 2.25H6a2.25 2.25 0 0 1-2.25-2.25V6ZM3.75 15.75A2.25 2.25 0 0 1 6 13.5h2.25a2.25 2.25 0 0 1 2.25 2.25V18a2.25 2.25 0 0 1-2.25 2.25H6A2.25 2.25 0 0 1 3.75 18v-2.25ZM13.5 6a2.25 2.25 0 0 1 2.25-2.25H18A2.25 2.25 0 0 1 20.25 6v2.25A2.25 2.25 0 0 1 18 10.5h-2.25a2.25 2.25 0 0 1-2.25-2.25V6ZM13.5 15.75a2.25 2.25 0 0 1 2.25-2.25H18a2.25 2.25 0 0 1 2.25 2.25V18A2.25 2.25 0 0 1 18 20.25h-2.25A2.25 2.25 0 0 1 13.5 18v-2.25Z" />`

// pluginNavItems builds the sidebar entries declared by loaded plugins.
// Visibility gating lives in plugins.Registry.VisibleMenus — the single gate
// shared with pin validation and search — so an item is pinnable exactly
// when it is visible.
func pluginNavItems(user *model.User) []NavItem {
	registry := plugins.GetRegistry()
	if registry == nil {
		return nil
	}
	byPlugin := make(map[string]*plugins.Plugin)
	for _, p := range registry.All() {
		byPlugin[p.Name] = p
	}
	var items []NavItem
	for _, menu := range registry.VisibleMenus(user) {
		if p, ok := byPlugin[menu.PluginName]; ok {
			items = append(items, pluginNavItem(p, menu))
		}
	}
	return items
}

func pluginNavItem(p *plugins.Plugin, menu plugins.Menu) NavItem {
	inner := menu.IconSVG
	if inner == "" {
		inner = iconPlugin
	}
	return NavItem{
		URL:      menu.URL,
		Label:    menu.Label,
		Icon:     navIcon(inner),
		External: isExternalNavURL(menu.URL),
	}
}

func isExternalNavURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// NavSection is one titled group of sidebar entries. Every section collapses;
// Workspace, the everyday destinations, starts open and the others closed,
// and a section opens by itself when it holds the current page.
type NavSection struct {
	Key         string
	Label       string
	Items       []NavItem
	DefaultOpen bool
	Active      bool // holds the current page
}

const (
	navWorkspace = "workspace"
	navBuild     = "build"
	navAdmin     = "admin"
	navPlugins   = "plugins"
)

// buildNav returns the visible nav items grouped into sections, in render
// order: Workspace (what people use day to day), Build (what spaces are made
// from, their volumes, and the automation around them), Admin (people, access and the
// system) and Extensions (plugin pages). Visibility mirrors the permission
// gates of each page. Sections with nothing visible are left out.
//
// auditAvailable is whether the audit-log feature is usable at all in this
// deployment (storage present + not routed externally); it's passed in rather
// than read from the database here so the function stays pure and testable.
func buildNav(user *model.User, cfg *config.ServerConfig, auditAvailable bool) []NavSection {
	leaf := cfg.LeafNode
	useSpaces := user.HasPermission(model.PermissionUseSpaces) || user.HasPermission(model.PermissionManageSpaces)
	useTunnels := user.HasPermission(model.PermissionUseTunnels) && cfg.ListenTunnel != ""
	manageVolumes := user.HasPermission(model.PermissionManageVolumes)
	manageTemplates := user.HasPermission(model.PermissionManageTemplates)
	manageVariables := user.HasPermission(model.PermissionManageVariables)
	manageScripts := user.HasPermission(model.PermissionManageScripts) || user.HasPermission(model.PermissionManageOwnScripts)
	manageEvents := user.HasPermission(model.PermissionManageEvents) || user.HasPermission(model.PermissionManageGlobalEvents)
	manageSkills := user.HasPermission(model.PermissionManageGlobalSkills) || user.HasPermission(model.PermissionManageOwnSkills)
	manageCommands := user.HasPermission(model.PermissionManageGlobalSlashCommands) || user.HasPermission(model.PermissionManageOwnSlashCommands)
	manageMCP := user.HasPermission(model.PermissionManageMCPServers)
	manageStacks := user.HasPermission(model.PermissionManageStackDefinitions) ||
		user.HasPermission(model.PermissionManageOwnStackDefinitions) ||
		user.HasPermission(model.PermissionUseStackDefinitions)
	manageUsers := user.HasPermission(model.PermissionManageUsers)
	manageGroups := user.HasPermission(model.PermissionManageGroups)
	manageRoles := user.HasPermission(model.PermissionManageRoles)
	viewCluster := user.HasPermission(model.PermissionClusterInfo) && cfg.Cluster.AdvertiseAddr != ""
	viewAudit := user.HasPermission(model.PermissionViewAuditLogs) && auditAvailable
	viewPlugins := user.HasPermission(model.PermissionViewPlugins) && plugins.GetRegistry().Present()

	var workspace, build, admin []NavItem
	if useSpaces || leaf {
		workspace = append(workspace, nav("/spaces", "Spaces", iconSpaces))
	}
	if filestore.CanUseFilesPage(filestore.Get(), user) {
		workspace = append(workspace, nav("/files", "Files", iconFiles))
	}
	if useTunnels && !leaf {
		workspace = append(workspace, nav("/tunnels", "Tunnels", iconTunnels))
	}
	if !cfg.UI.HideAPITokens {
		workspace = append(workspace, nav("/api-tokens", "API Tokens", iconTokens))
	}

	// A leaf node manages its own templates and variables, the main thing
	// done there, so they sit with the everyday items.
	if leaf {
		workspace = append(workspace, nav("/templates", "Templates", iconTemplates))
		workspace = append(workspace, nav("/variables", "Variables", iconVariables))
	} else {
		if manageTemplates {
			build = append(build, nav("/templates", "Templates", iconTemplates))
		}
		if manageVariables {
			build = append(build, nav("/variables", "Variables", iconVariables))
		}
	}
	if manageStacks {
		build = append(build, nav("/stacks", "Stack Templates", iconStacks))
	}
	if manageVolumes || leaf {
		build = append(build, nav("/volumes", "Volumes", iconVolumes))
	}
	if manageScripts {
		build = append(build, nav("/scripts", "Scripts", iconScripts))
	}
	if manageEvents {
		build = append(build, nav("/events", "Events", iconEvents))
	}
	if manageSkills {
		build = append(build, nav("/skills", "Skills", iconSkills))
	}
	if manageCommands {
		build = append(build, nav("/commands", "Slash Commands", iconCommands))
	}
	if manageMCP || leaf {
		build = append(build, nav("/mcp-servers", "MCP Servers", iconMCP))
	}

	if manageUsers && !leaf {
		admin = append(admin, nav("/users", "Users", iconUsers))
	}
	if manageGroups && !leaf {
		admin = append(admin, nav("/groups", "Groups", iconGroups))
	}
	if manageRoles && !leaf {
		admin = append(admin, nav("/roles", "Roles", iconRoles))
	}
	if viewAudit {
		admin = append(admin, nav("/audit-logs", "Audit Logs", iconAudit))
	}
	if viewCluster && !leaf {
		admin = append(admin, nav("/cluster-info", "Cluster Info", iconCluster))
	}
	// The plugin inventory is admin-only, like the page itself, and only
	// shown when the plugins path produced anything (loaded, failed, or
	// warnings); with no plugins the surface is hidden entirely.
	if viewPlugins && !leaf {
		admin = append(admin, nav("/plugins", "Plugins", iconPlugin))
	}

	sections := []NavSection{
		{Key: navWorkspace, Label: "Workspace", Items: workspace, DefaultOpen: true},
		{Key: navBuild, Label: "Build", Items: build},
		{Key: navAdmin, Label: "Admin", Items: admin},
		// Plugin-declared menu items, permission/group gated per item.
		{Key: navPlugins, Label: "Extensions", Items: pluginNavItems(user)},
	}
	return nonEmptySections(sections)
}

func nonEmptySections(sections []NavSection) []NavSection {
	out := sections[:0]
	for _, s := range sections {
		if len(s.Items) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// resolveNav computes the final sidebar state for the current user: the
// starred items, in the user's order, then the sections with the starred
// items taken out of them. A section left empty is not shown, so starring
// everything a section held hides its heading too.
//
// The item owning requestPath is marked Active (for aria-current), and its
// section opens.
func resolveNav(user *model.User, cfg *config.ServerConfig, auditAvailable bool, requestPath string) (starred []NavItem, sections []NavSection) {
	sections = buildNav(user, cfg, auditAvailable)

	// Build a URL→item lookup over everything the user can see.
	visible := make(map[string]NavItem)
	for _, s := range sections {
		for _, it := range s.Items {
			visible[it.URL] = it
		}
	}

	// Resolve the stored pin order, dropping stale/hidden/duplicate entries so
	// a revoked permission or renamed route can't leave dead pins around.
	pinned := make(map[string]bool)
	for _, url := range user.GetNavStarred() {
		if it, ok := visible[url]; ok && !pinned[url] {
			it.Starred = true
			starred = append(starred, it)
			pinned[url] = true
		}
	}

	for i := range sections {
		kept := make([]NavItem, 0, len(sections[i].Items))
		for _, it := range sections[i].Items {
			if !pinned[it.URL] {
				kept = append(kept, it)
			}
		}
		sections[i].Items = kept
	}
	sections = nonEmptySections(sections)

	lists := [][]NavItem{starred}
	for _, s := range sections {
		lists = append(lists, s.Items)
	}
	if owner, ok := navPathOwner(requestPath, lists...); ok {
		for i := range starred {
			starred[i].Active = starred[i].URL == owner.URL
		}
		for i := range sections {
			for j := range sections[i].Items {
				if sections[i].Items[j].URL == owner.URL {
					sections[i].Items[j].Active = true
					sections[i].Active = true
				}
			}
		}
	}
	return starred, sections
}

// navPathOwner returns the item whose URL owns requestPath — an exact match
// or a whole-segment prefix ("/spaces" owns "/spaces/123") — preferring the
// longest URL when several items match. The longest-match rule keeps nested
// URLs honest: a plugin page under /plugins/<name> is owned by the plugin's
// own menu item, not the shorter /plugins inventory entry.
func navPathOwner(requestPath string, lists ...[]NavItem) (NavItem, bool) {
	var best NavItem
	found := false
	for _, list := range lists {
		for _, it := range list {
			if requestPath == it.URL || strings.HasPrefix(requestPath, it.URL+"/") {
				if !found || len(it.URL) > len(best.URL) {
					best = it
					found = true
				}
			}
		}
	}
	return best, found
}

// applyNav builds the sidebar state for the request and merges it into the
// template data map.
func applyNav(user *model.User, cfg *config.ServerConfig, requestPath string, data map[string]interface{}) {
	auditAvailable := database.GetInstance().HasAuditLog() && cfg.Audit.Routing != "external"
	starred, sections := resolveNav(user, cfg, auditAvailable, requestPath)
	data["navStarred"] = starred
	data["navSections"] = sections
}
