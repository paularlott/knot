package model

import "github.com/paularlott/knot/internal/config"

// NavPage is a sidebar navigation destination (a page), without any icon or
// top/More grouping — just enough for the global-search "pages" group.
type NavPage struct {
	URL   string
	Label string
}

// VisibleNavPages returns the sidebar pages the given user may see, in sidebar
// order. The visibility gates mirror web/nav.go buildNav exactly (combined
// across the top and "More" sections), so a page appears here iff it appears
// in the user's menu. auditAvailable is whether the audit-log feature is
// usable in this deployment (storage present + not routed externally) — passed
// in so this function stays pure and testable.
func VisibleNavPages(user *User, cfg *config.ServerConfig, auditAvailable bool) []NavPage {
	leaf := cfg.LeafNode
	useSpaces := user.HasPermission(PermissionUseSpaces) || user.HasPermission(PermissionManageSpaces)
	useTunnels := user.HasPermission(PermissionUseTunnels) && cfg.ListenTunnel != ""

	all := []struct {
		url, label string
		visible    bool
	}{
		{"/spaces", "Spaces", useSpaces || leaf},
		{"/tunnels", "Tunnels", useTunnels && !leaf},
		{"/api-tokens", "API Tokens", !cfg.UI.HideAPITokens},
		{"/volumes", "Volumes", user.HasPermission(PermissionManageVolumes) || leaf},
		{"/templates", "Templates", user.HasPermission(PermissionManageTemplates) || leaf},
		{"/variables", "Variables", user.HasPermission(PermissionManageVariables) || leaf},
		{"/stacks", "Stack Templates", user.HasPermission(PermissionManageStackDefinitions) || user.HasPermission(PermissionManageOwnStackDefinitions) || user.HasPermission(PermissionUseStackDefinitions)},
		{"/scripts", "Scripts", user.HasPermission(PermissionManageScripts) || user.HasPermission(PermissionManageOwnScripts)},
		{"/events", "Events", user.HasPermission(PermissionManageEvents) || user.HasPermission(PermissionManageGlobalEvents)},
		{"/skills", "Skills", user.HasPermission(PermissionManageGlobalSkills) || user.HasPermission(PermissionManageOwnSkills)},
		{"/commands", "Slash Commands", user.HasPermission(PermissionManageGlobalSlashCommands) || user.HasPermission(PermissionManageOwnSlashCommands)},
		{"/mcp-servers", "MCP Servers", user.HasPermission(PermissionManageMCPServers) || leaf},
		{"/users", "Users", user.HasPermission(PermissionManageUsers) && !leaf},
		{"/groups", "Groups", user.HasPermission(PermissionManageGroups) && !leaf},
		{"/roles", "Roles", user.HasPermission(PermissionManageRoles) && !leaf},
		{"/audit-logs", "Audit Logs", user.HasPermission(PermissionViewAuditLogs) && auditAvailable},
		{"/cluster-info", "Cluster Info", user.HasPermission(PermissionClusterInfo) && cfg.Cluster.AdvertiseAddr != "" && !leaf},
	}

	pages := make([]NavPage, 0, len(all))
	for _, m := range all {
		if m.visible {
			pages = append(pages, NavPage{URL: m.url, Label: m.label})
		}
	}
	return pages
}
