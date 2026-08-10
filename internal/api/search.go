package api

import (
	"net/http"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
)

// searchLimitPerType caps each result group so a single keystroke never
// returns unbounded lists — the palette shows a handful per type with the
// option to open the full page.
const searchLimitPerType = 5

// HandleSearch performs a permission-, ownership- and zone-scoped keyword
// search across the entity types the current user may see. Matching is a
// case-insensitive substring match on each entity's primary name. Each result
// group is capped at searchLimitPerType entries; empty groups are omitted.
//
// Scoping mirrors the sidebar visibility gates:
//   - spaces are the current user's own plus any shared with them;
//   - admin entities (templates, variables, volumes, users, groups, roles,
//     stacks, event sinks) appear only when the user holds the manage
//     permission for that type;
//   - personal entities (scripts, skills, slash commands, MCP servers, tokens)
//     are scoped to the user's own (plus global ones where the list endpoint
//     exposes them);
//   - leaf nodes get the same reduced set as the sidebar (no users/groups/roles).
func HandleSearch(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	out := &apiclient.SearchResults{Query: q}
	if q == "" {
		rest.WriteResponse(http.StatusOK, w, r, out)
		return
	}

	needle := strings.ToLower(q)
	match := func(name string) bool {
		return name != "" && strings.Contains(strings.ToLower(name), needle)
	}

	db := database.GetInstance()
	cfg := config.GetServerConfig()
	leaf := cfg.LeafNode

	// --- Spaces: the current user's own + shared spaces (personal palette). ---
	if user.HasPermission(model.PermissionUseSpaces) || user.HasPermission(model.PermissionManageSpaces) || leaf {
		spaces, _ := db.GetSpacesForUser(user.Id)
		for _, s := range spaces {
			if s.IsDeleted || !match(s.Name) {
				continue
			}
			out.Spaces = append(out.Spaces, apiclient.SearchHit{Id: s.Id, Name: s.Name, Description: s.Description})
			if len(out.Spaces) >= searchLimitPerType {
				break
			}
		}
	}

	// --- Templates ---
	if user.HasPermission(model.PermissionManageTemplates) || leaf {
		if list, err := db.GetTemplates(); err == nil {
			for _, t := range list {
				if t.IsDeleted || !match(t.Name) {
					continue
				}
				out.Templates = append(out.Templates, apiclient.SearchHit{Id: t.Id, Name: t.Name, Description: t.Description})
				if len(out.Templates) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- Template Variables ---
	// Same variable name can exist for different zones (or as local), so show
	// the scope + flags as the description to disambiguate duplicates.
	if user.HasPermission(model.PermissionManageVariables) || leaf {
		if list, err := db.GetTemplateVars(); err == nil {
			for _, v := range list {
				if v.IsDeleted || !match(v.Name) {
					continue
				}
				scope := "All zones"
				if v.Local {
					scope = "Local"
				} else if len(v.Zones) > 0 {
					scope = strings.Join(v.Zones, ", ")
				}
				if v.Protected {
					scope += " · Protected"
				}
				if v.Restricted {
					scope += " · Restricted"
				}
				out.Variables = append(out.Variables, apiclient.SearchHit{Id: v.Id, Name: v.Name, Description: scope})
				if len(out.Variables) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- Volumes ---
	if user.HasPermission(model.PermissionManageVolumes) || leaf {
		if list, err := db.GetVolumes(); err == nil {
			for _, v := range list {
				if v.IsDeleted || !match(v.Name) {
					continue
				}
				out.Volumes = append(out.Volumes, apiclient.SearchHit{Id: v.Id, Name: v.Name})
				if len(out.Volumes) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- Stack Definitions ---
	if user.HasPermission(model.PermissionManageStackDefinitions) ||
		user.HasPermission(model.PermissionManageOwnStackDefinitions) ||
		user.HasPermission(model.PermissionUseStackDefinitions) {
		if list, err := db.GetStackDefinitions(); err == nil {
			for _, s := range list {
				if s.IsDeleted || !match(s.Name) {
					continue
				}
				out.Stacks = append(out.Stacks, apiclient.SearchHit{Id: s.Id, Name: s.Name, Description: s.Description})
				if len(out.Stacks) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- Scripts (global + own) ---
	if user.HasPermission(model.PermissionManageScripts) || user.HasPermission(model.PermissionManageOwnScripts) {
		if list, err := db.GetScripts(); err == nil {
			for _, s := range list {
				if s.IsDeleted || !match(s.Name) {
					continue
				}
				out.Scripts = append(out.Scripts, apiclient.SearchHit{Id: s.Id, Name: s.Name, Description: s.Description})
				if len(out.Scripts) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- Skills ---
	if user.HasPermission(model.PermissionManageGlobalSkills) || user.HasPermission(model.PermissionManageOwnSkills) {
		if list, err := db.GetSkills(); err == nil {
			for _, s := range list {
				if s.IsDeleted || !match(s.Name) {
					continue
				}
				out.Skills = append(out.Skills, apiclient.SearchHit{Id: s.Id, Name: s.Name, Description: s.Description})
				if len(out.Skills) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- Slash Commands ---
	if user.HasPermission(model.PermissionManageGlobalSlashCommands) || user.HasPermission(model.PermissionManageOwnSlashCommands) {
		if list, err := db.GetCommands(); err == nil {
			for _, c := range list {
				if c.IsDeleted || !match(c.Name) {
					continue
				}
				out.Commands = append(out.Commands, apiclient.SearchHit{Id: c.Id, Name: c.Name, Description: c.Description})
				if len(out.Commands) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- MCP Servers (own) ---
	if user.HasPermission(model.PermissionManageMCPServers) || leaf {
		if list, err := db.GetMCPServersByUser(user.Id); err == nil {
			for _, m := range list {
				if m.IsDeleted || !match(m.Namespace) {
					continue
				}
				out.MCPServers = append(out.MCPServers, apiclient.SearchHit{Id: m.Id, Name: m.Namespace})
				if len(out.MCPServers) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- Event Sinks ---
	if user.HasPermission(model.PermissionManageEvents) || user.HasPermission(model.PermissionManageGlobalEvents) {
		if list, err := db.GetEventSinks(); err == nil {
			for _, e := range list {
				if e.IsDeleted || !match(e.Name) {
					continue
				}
				out.EventSinks = append(out.EventSinks, apiclient.SearchHit{Id: e.Id, Name: e.Name, Description: e.Description})
				if len(out.EventSinks) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- Users (admin, non-leaf) ---
	if user.HasPermission(model.PermissionManageUsers) && !leaf {
		if list, err := db.GetUsers(); err == nil {
			for _, u := range list {
				if u.IsDeleted || (!match(u.Username) && !match(u.Email)) {
					continue
				}
				out.Users = append(out.Users, apiclient.SearchHit{Id: u.Id, Name: u.Username, Description: u.Email})
				if len(out.Users) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- Groups (admin, non-leaf) ---
	if user.HasPermission(model.PermissionManageGroups) && !leaf {
		if list, err := db.GetGroups(); err == nil {
			for _, g := range list {
				if g.IsDeleted || !match(g.Name) {
					continue
				}
				out.Groups = append(out.Groups, apiclient.SearchHit{Id: g.Id, Name: g.Name})
				if len(out.Groups) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- Roles (admin, non-leaf) ---
	if user.HasPermission(model.PermissionManageRoles) && !leaf {
		if list, err := db.GetRoles(); err == nil {
			for _, role := range list {
				if role.IsDeleted || !match(role.Name) {
					continue
				}
				out.Roles = append(out.Roles, apiclient.SearchHit{Id: role.Id, Name: role.Name})
				if len(out.Roles) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- Tokens (own) ---
	if !cfg.UI.HideAPITokens {
		if list, err := db.GetTokensForUser(user.Id); err == nil {
			for _, t := range list {
				if t.IsDeleted || !match(t.Name) {
					continue
				}
				out.Tokens = append(out.Tokens, apiclient.SearchHit{Id: t.Id, Name: t.Name})
				if len(out.Tokens) >= searchLimitPerType {
					break
				}
			}
		}
	}

	// --- Pages (navigation destinations) ---
	// Same visibility gates as the sidebar (model.VisibleNavPages), so a page
	// only appears if the user would see it in the menu.
	auditAvailable := db.HasAuditLog() && cfg.Audit.Routing != "external"
	for _, p := range model.VisibleNavPages(user, cfg, auditAvailable) {
		if match(p.Label) || match(p.URL) {
			out.Pages = append(out.Pages, apiclient.SearchHit{Id: p.URL, Name: p.Label})
			if len(out.Pages) >= searchLimitPerType {
				break
			}
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, out)
}
