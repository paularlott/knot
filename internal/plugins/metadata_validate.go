package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// pluginEnvLibraries are the library names knot registers in plugin handler
// environments (see internal/service registerPluginLibraries), used both by
// that env and by metadata dependency resolution at load time. Keep the two
// lists in step. Deliberately absent: requests and scriptling.wait_for
// (outbound networking) and scriptling.container / scriptling.nomad
// (runtime access — drivers only).
var pluginEnvLibraries = map[string]bool{
	// stdlib (stdlib.RegisterAll)
	"json": true, "re": true, "math": true, "time": true, "datetime": true,
	"itertools": true, "random": true, "string": true, "collections": true,
	"functools": true, "heapq": true, "bisect": true,
	"base64": true, "binascii": true, "hashlib": true, "hmac": true,
	"uuid": true, "urllib": true, "statistics": true, "types": true,
	"weakref": true, "array": true,

	// base extended libraries
	"secrets":                       true,
	"html.parser":                   true,
	"yaml":                          true,
	"toml":                          true,
	"logging":                       true,
	"shlex":                         true,
	"scriptling.csv":                true,
	"scriptling.xml":                true,
	"scriptling.template.html":      true,
	"scriptling.template.text":      true,
	"scriptling.ai":                 true,
	"scriptling.ai.agent":           true,
	"scriptling.ai.agent.interact":  true,
	"scriptling.ai.tools":           true,
	"scriptling.ai.memory":          true,
	"scriptling.similarity":         true,
	"scriptling.mcp":                true,
	"scriptling.toon":               true,
	"scriptling.mcp.tool":           true,
	"scriptling.messaging.telegram": true,
	"scriptling.messaging.discord":  true,
	"scriptling.messaging.slack":    true,

	// system access (scoped to the plugin folder in the env)
	"os": true, "os.path": true, "pathlib": true, "glob": true,
	"tempfile": true, "shutil": true, "zipfile": true, "tarfile": true,
	"fs": true, "sys": true, "subprocess": true,
	"scriptling.grep": true, "scriptling.find": true, "scriptling.sed": true,
	"scriptling.provision.file":  true,
	"scriptling.provision.fetch": true,
}

// toolKnotKeys are the keys accepted at the top of [tool.knot]. Unknown keys
// are load errors: a typo should fail loudly, not silently do nothing.
var toolKnotKeys = map[string]bool{
	"version":        true,
	"description":    true,
	"permissions":    true,
	"logo_light":     true,
	"logo_dark":      true,
	"menus":          true,
	"pages":          true,
	"handlers":       true,
	"field_handlers": true,
}

var fieldHandlerKeys = map[string]bool{
	"label": true, "handler": true, "permission": true, "group": true,
}

var handlerDeclKeys = map[string]bool{
	"handler": true, "permission": true, "group": true,
}

var pageKeys = map[string]bool{
	"path":       true,
	"handler":    true,
	"label":      true,
	"menu_label": true,
	"permission": true,
	"group":      true,
	"icon":       true,
	"default":    true,
}

// handlerNameRe accepts a function name or module.function reference.
var handlerNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// ValidHandlerName reports whether name is a legal handler reference (a
// function name or module.function path) — the same shape [tool.knot] pages
// and field_handlers declare, and what handler-URL dispatch accepts.
func ValidHandlerName(name string) bool {
	return handlerNameRe.MatchString(name)
}

var menuKeys = map[string]bool{
	"label":      true,
	"url":        true,
	"permission": true,
	"group":      true,
	"icon":       true,
}

// parseToolKnot validates the [tool.knot] table into a Plugin. pluginDir is
// the folder containing the entry file (the plugins root for single-file
// plugins). All validation is static — no plugin code runs.
func parseToolKnot(name, pluginDir string, singleFile bool, table map[string]any) (*Plugin, error) {
	p := &Plugin{Name: name}

	for key := range table {
		if !toolKnotKeys[key] {
			return nil, fmt.Errorf("[tool.knot]: unknown key %q", key)
		}
	}

	if v, ok := table["version"]; ok {
		s, ok := v.(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("[tool.knot]: version must be a non-empty string")
		}
		p.Version = s
	}
	if v, ok := table["description"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("[tool.knot]: description must be a string")
		}
		p.Description = s
	}

	// Permissions: a plain list of ids, qualified at load.
	declared := map[string]bool{}
	if v, ok := table["permissions"]; ok {
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("[tool.knot]: permissions must be a list of ids")
		}
		for _, entry := range list {
			id, ok := entry.(string)
			if !ok || !permissionIdRe.MatchString(id) {
				return nil, fmt.Errorf("[tool.knot]: permissions entries must be ids matching [a-z0-9_]+")
			}
			if declared[id] {
				return nil, fmt.Errorf("[tool.knot]: duplicate permission %q", id)
			}
			declared[id] = true
			p.Permissions = append(p.Permissions, PermissionDecl{
				Id:    QualifiedPermission(name, id),
				Label: id,
			})
		}
	}

	// Logos: relative paths that must stay inside the plugin folder and
	// exist. Single-file plugins have no folder to carry assets. One or both
	// may be declared: a single logo serves both themes (copied to the other
	// slot), so everything downstream always sees a complete pair.
	for key, field := range map[string]*string{"logo_light": &p.LogoLight, "logo_dark": &p.LogoDark} {
		v, ok := table[key]
		if !ok {
			continue
		}
		rel, ok := v.(string)
		if !ok || rel == "" {
			return nil, fmt.Errorf("[tool.knot]: %s must be a non-empty relative path", key)
		}
		if singleFile {
			return nil, fmt.Errorf("[tool.knot]: %s: single-file plugins cannot carry assets; use a folder plugin", key)
		}
		if err := validateAssetPath(pluginDir, rel); err != nil {
			return nil, fmt.Errorf("[tool.knot]: %s: %w", key, err)
		}
		*field = rel
	}
	if p.LogoLight == "" {
		p.LogoLight = p.LogoDark
	}
	if p.LogoDark == "" {
		p.LogoDark = p.LogoLight
	}
	// A declared logo pair is the site-logo claim.
	p.SiteLogo = p.LogoLight != ""

	// Menus.
	if v, ok := table["menus"]; ok {
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("[tool.knot]: menus must be a list of tables")
		}
		seenURLs := map[string]bool{}
		for i, entry := range list {
			menuTable, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("[tool.knot]: menus[%d] must be a table", i)
			}
			menu, err := parseMenu(name, i, menuTable, declared)
			if err != nil {
				return nil, err
			}
			if seenURLs[menu.URL] {
				return nil, fmt.Errorf("[tool.knot]: menus[%d]: duplicate url %q", i, menu.URL)
			}
			seenURLs[menu.URL] = true
			p.Menus = append(p.Menus, menu)
		}
	}

	for i := range p.Menus {
		if err := p.loadIcon(pluginDir, singleFile, &p.Menus[i].Icon, &p.Menus[i].IconSVG, fmt.Sprintf("menus[%d]", i)); err != nil {
			return nil, err
		}
	}

	// Pages.
	if v, ok := table["pages"]; ok {
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("[tool.knot]: pages must be a list of tables")
		}
		seenPaths := map[string]bool{}
		defaultSeen := false
		for i, entry := range list {
			pageTable, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("[tool.knot]: pages[%d] must be a table", i)
			}
			page, err := parsePage(name, i, pageTable, declared)
			if err != nil {
				return nil, err
			}
			if seenPaths[page.Path] {
				return nil, fmt.Errorf("[tool.knot]: pages[%d]: duplicate path %q", i, page.Path)
			}
			if page.Default && defaultSeen {
				return nil, fmt.Errorf("[tool.knot]: pages[%d]: only one page per plugin may set default", i)
			}
			if page.Default {
				defaultSeen = true
			}
			seenPaths[page.Path] = true
			p.Pages = append(p.Pages, page)
		}
	}

	if v, ok := table["field_handlers"]; ok {
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("[tool.knot]: field_handlers must be a list of tables")
		}
		seenIds := map[string]bool{}
		for i, entry := range list {
			entryTable, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("[tool.knot]: field_handlers[%d] must be a table", i)
			}
			for key := range entryTable {
				if !fieldHandlerKeys[key] {
					return nil, fmt.Errorf("[tool.knot]: field_handlers[%d]: unknown key %q", i, key)
				}
			}
			// The handler function name is the handler's identity: it is
			// unique within the plugin, and the qualified form
			// plugin.<name>.<handler> is what templates bind to.
			label, _ := entryTable["label"].(string)
			handler, _ := entryTable["handler"].(string)
			if handler == "" {
				return nil, fmt.Errorf("[tool.knot]: field_handlers[%d]: handler is required", i)
			}
			if label == "" {
				label = handler
			}
			qualified := QualifiedPermission(name, handler)
			if seenIds[qualified] {
				return nil, fmt.Errorf("[tool.knot]: field_handlers[%d]: duplicate handler %q", i, handler)
			}
			seenIds[qualified] = true
			field := FieldHandler{Id: qualified, Label: label, Handler: handler}
			if v, ok := entryTable["permission"]; ok {
				id, ok := v.(string)
				if !ok || !permissionIdRe.MatchString(id) {
					return nil, fmt.Errorf("[tool.knot]: field_handlers[%d]: permission must be an id matching [a-z0-9_]+", i)
				}
				if !declared[id] {
					return nil, fmt.Errorf("[tool.knot]: field_handlers[%d]: permission %q is not declared in [tool.knot] permissions", i, id)
				}
				field.Permission = QualifiedPermission(name, id)
			}
			if v, ok := entryTable["group"]; ok {
				group, ok := v.(string)
				if !ok || group == "" {
					return nil, fmt.Errorf("[tool.knot]: field_handlers[%d]: group must be a non-empty string", i)
				}
				field.Group = group
			}
			p.FieldHandlers = append(p.FieldHandlers, field)
		}
	}
	// Handler gates: [[tool.knot.handlers]] entries. A declared gate is the
	// handler's own permission/group wherever it is called (overriding the
	// calling page's), and the declaration opts the handler into
	// plugin-root addressability.
	if v, ok := table["handlers"]; ok {
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("[tool.knot]: handlers must be a list of tables")
		}
		seen := map[string]bool{}
		for i, entry := range list {
			entryTable, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("[tool.knot]: handlers[%d] must be a table", i)
			}
			for key := range entryTable {
				if !handlerDeclKeys[key] {
					return nil, fmt.Errorf("[tool.knot]: handlers[%d]: unknown key %q", i, key)
				}
			}
			hname, _ := entryTable["handler"].(string)
			if hname == "" || !handlerNameRe.MatchString(hname) {
				return nil, fmt.Errorf("[tool.knot]: handlers[%d]: handler is required and must be a function name or module.function", i)
			}
			if seen[hname] {
				return nil, fmt.Errorf("[tool.knot]: handlers[%d]: duplicate handler %q", i, hname)
			}
			seen[hname] = true
			decl := Handler{Handler: hname}
			if v, ok := entryTable["permission"]; ok {
				id, ok := v.(string)
				if !ok || !permissionIdRe.MatchString(id) {
					return nil, fmt.Errorf("[tool.knot]: handlers[%d]: permission must be an id matching [a-z0-9_]+", i)
				}
				if !declared[id] {
					return nil, fmt.Errorf("[tool.knot]: handlers[%d]: permission %q is not declared in [tool.knot] permissions", i, id)
				}
				decl.Permission = QualifiedPermission(name, id)
			}
			if v, ok := entryTable["group"]; ok {
				group, ok := v.(string)
				if !ok || group == "" {
					return nil, fmt.Errorf("[tool.knot]: handlers[%d]: group must be a non-empty string", i)
				}
				decl.Group = group
			}
			p.Handlers = append(p.Handlers, decl)
		}
	}

	// Load page icons first, so a page's sidebar item can inherit its icon.
	// A page with a menu_label contributes that item for itself; the URL is
	// built from the plugin name directly — PluginName is set on pages only
	// after parsing completes.
	for i := range p.Pages {
		if err := p.loadIcon(pluginDir, singleFile, &p.Pages[i].Icon, &p.Pages[i].IconSVG, fmt.Sprintf("pages[%d]", i)); err != nil {
			return nil, err
		}
		if p.Pages[i].MenuLabel != "" {
			p.Menus = append(p.Menus, Menu{
				Label:      p.Pages[i].MenuLabel,
				URL:        "/plugins/" + name + p.Pages[i].Path,
				Permission: p.Pages[i].Permission,
				Group:      p.Pages[i].Group,
				Icon:       p.Pages[i].Icon,
				IconSVG:    p.Pages[i].IconSVG,
			})
		}
	}

	return p, nil
}

// iconMaxBytes bounds an icon asset: it is inlined into every rendered page
// that shows the sidebar.
const iconMaxBytes = 64 * 1024

// loadIcon validates and loads an icon asset: a relative .svg inside the
// plugin folder, size-capped, whose inner markup is safe to render inline.
// The inner markup is extracted and stored; knot wraps it in the site's <svg>
// attributes, so a currentColor-stroked icon themes like the built-ins.
func (p *Plugin) loadIcon(pluginDir string, singleFile bool, decl *string, inner *string, where string) error {
	if *decl == "" {
		return nil
	}
	if singleFile {
		return fmt.Errorf("[tool.knot]: %s: icon: single-file plugins cannot carry assets; use a folder plugin", where)
	}
	if err := validateAssetPath(pluginDir, *decl); err != nil {
		return fmt.Errorf("[tool.knot]: %s: icon: %w", where, err)
	}
	if strings.ToLower(filepath.Ext(*decl)) != ".svg" {
		return fmt.Errorf("[tool.knot]: %s: icon must be an .svg asset in the plugin folder", where)
	}
	content, err := os.ReadFile(filepath.Join(pluginDir, filepath.FromSlash(filepath.Clean(*decl))))
	if err != nil {
		return fmt.Errorf("[tool.knot]: %s: icon: %w", where, err)
	}
	if len(content) > iconMaxBytes {
		return fmt.Errorf("[tool.knot]: %s: icon exceeds %d bytes", where, iconMaxBytes)
	}
	markup, err := sanitizeIconSVG(string(content))
	if err != nil {
		return fmt.Errorf("[tool.knot]: %s: icon %q: %w", where, *decl, err)
	}
	*inner = markup
	return nil
}

// iconForbidden matches inline-SVG hazards: script blocks, event-handler
// attributes, and any element or attribute that can reference outside
// content. Icons are rendered inline, so anything beyond passive shapes is
// refused at load.
var iconForbidden = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<script`),
	regexp.MustCompile(`(?i)\son[a-z]+\s*=`),
	regexp.MustCompile(`(?i)javascript:`),
	regexp.MustCompile(`(?i)<\s*(use|image|foreignObject|animate|set|iframe|embed|object)`),
	regexp.MustCompile(`(?i)\s(xlink:href|href)\s*=`),
}

// sanitizeIconSVG extracts the inner markup of the root <svg> element and
// verifies it contains none of the forbidden constructs.
func sanitizeIconSVG(content string) (string, error) {
	start := regexp.MustCompile(`(?is)<svg[^>]*>`).FindStringIndex(content)
	end := regexp.MustCompile(`(?is)</svg\s*>`).FindStringIndex(content)
	if start == nil || end == nil || end[0] < start[1] {
		return "", fmt.Errorf("not a valid SVG document")
	}
	inner := strings.TrimSpace(content[start[1]:end[0]])
	if inner == "" {
		return "", fmt.Errorf("SVG has no content")
	}
	for _, re := range iconForbidden {
		if re.MatchString(inner) {
			return "", fmt.Errorf("contains a construct that is not allowed in icons (%q)", re.String())
		}
	}
	return inner, nil
}

func parseMenu(pluginName string, index int, table map[string]any, declared map[string]bool) (Menu, error) {
	menu := Menu{}
	for key := range table {
		if !menuKeys[key] {
			return menu, fmt.Errorf("[tool.knot]: menus[%d]: unknown key %q", index, key)
		}
	}
	if v, ok := table["label"]; ok {
		s, _ := v.(string)
		if strings.TrimSpace(s) == "" {
			return menu, fmt.Errorf("[tool.knot]: menus[%d]: label must be a non-empty string", index)
		}
		menu.Label = s
	} else {
		return menu, fmt.Errorf("[tool.knot]: menus[%d]: label is required", index)
	}
	if v, ok := table["url"]; ok {
		s, _ := v.(string)
		if !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
			return menu, fmt.Errorf("[tool.knot]: menus[%d]: url must start with \"/\", \"http://\" or \"https://\"", index)
		}
		menu.URL = s
	} else {
		return menu, fmt.Errorf("[tool.knot]: menus[%d]: url is required", index)
	}
	if v, ok := table["permission"]; ok {
		id, _ := v.(string)
		if !declared[id] {
			return menu, fmt.Errorf("[tool.knot]: menus[%d]: permission %q is not declared in [tool.knot] permissions", index, id)
		}
		menu.Permission = QualifiedPermission(pluginName, id)
	}
	if v, ok := table["group"]; ok {
		s, _ := v.(string)
		if s == "" {
			return menu, fmt.Errorf("[tool.knot]: menus[%d]: group must be a non-empty string", index)
		}
		menu.Group = s
	}
	if v, ok := table["icon"]; ok {
		s, _ := v.(string)
		if s == "" {
			return menu, fmt.Errorf("[tool.knot]: menus[%d]: icon must be a path to an .svg asset in the plugin folder", index)
		}
		menu.Icon = s
	}
	return menu, nil
}

// parsePage validates one [[tool.knot.pages]] entry. Paths are the plugin's
// own namespace under /plugins/<name>; the assets prefix is reserved for the
// declared-logo file server.
func parsePage(pluginName string, index int, table map[string]any, declared map[string]bool) (Page, error) {
	page := Page{}
	for key := range table {
		if !pageKeys[key] {
			return page, fmt.Errorf("[tool.knot]: pages[%d]: unknown key %q", index, key)
		}
	}
	if v, ok := table["path"]; ok {
		s, _ := v.(string)
		if !strings.HasPrefix(s, "/") || s == "/" || strings.Contains(s, "..") || strings.ContainsAny(s, "\\\x00") {
			return page, fmt.Errorf("[tool.knot]: pages[%d]: path must start with %q and stay inside the plugin's namespace", index, "/")
		}
		if s == "/assets" || strings.HasPrefix(s, "/assets/") {
			return page, fmt.Errorf("[tool.knot]: pages[%d]: the /assets path is reserved", index)
		}
		page.Path = s
	} else {
		return page, fmt.Errorf("[tool.knot]: pages[%d]: path is required", index)
	}
	if v, ok := table["handler"]; ok {
		s, _ := v.(string)
		if !handlerNameRe.MatchString(s) {
			return page, fmt.Errorf("[tool.knot]: pages[%d]: handler must be a function name or module.function", index)
		}
		page.Handler = s
	} else {
		return page, fmt.Errorf("[tool.knot]: pages[%d]: handler is required", index)
	}
	if v, ok := table["label"]; ok {
		s, _ := v.(string)
		if strings.TrimSpace(s) == "" {
			return page, fmt.Errorf("[tool.knot]: pages[%d]: label must be a non-empty string", index)
		}
		page.Label = s
	}
	if v, ok := table["permission"]; ok {
		id, _ := v.(string)
		if !declared[id] {
			return page, fmt.Errorf("[tool.knot]: pages[%d]: permission %q is not declared in [tool.knot] permissions", index, id)
		}
		page.Permission = QualifiedPermission(pluginName, id)
	}
	if v, ok := table["group"]; ok {
		s, _ := v.(string)
		if s == "" {
			return page, fmt.Errorf("[tool.knot]: pages[%d]: group must be a non-empty string", index)
		}
		page.Group = s
	}
	if v, ok := table["icon"]; ok {
		s, _ := v.(string)
		if s == "" {
			return page, fmt.Errorf("[tool.knot]: pages[%d]: icon must be a non-empty name", index)
		}
		page.Icon = s
	}
	if v, ok := table["menu_label"]; ok {
		s, _ := v.(string)
		if strings.TrimSpace(s) == "" {
			return page, fmt.Errorf("[tool.knot]: pages[%d]: menu_label must be a non-empty string", index)
		}
		page.MenuLabel = s
	}
	if v, ok := table["default"]; ok {
		b, isBool := v.(bool)
		if !isBool {
			return page, fmt.Errorf("[tool.knot]: pages[%d]: default must be a boolean", index)
		}
		page.Default = b
	}
	return page, nil
}

// validateAssetPath checks that rel is a clean relative path that resolves
// inside pluginDir and refers to an existing regular file.
func validateAssetPath(pluginDir, rel string) error {
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") || strings.Contains(rel, "\\") {
		return fmt.Errorf("path %q must be relative and stay inside the plugin folder", rel)
	}
	clean := filepath.Clean(rel)
	if clean == "." || strings.HasPrefix(clean, "..") {
		return fmt.Errorf("path %q must stay inside the plugin folder", rel)
	}
	full := filepath.Join(pluginDir, clean)
	info, err := os.Stat(full)
	if err != nil {
		return fmt.Errorf("asset %q not found", rel)
	}
	if info.IsDir() {
		return fmt.Errorf("asset %q is a directory", rel)
	}
	return nil
}
