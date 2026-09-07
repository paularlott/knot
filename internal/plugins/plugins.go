// Package plugins implements knot's plugin system: a plugin is a single
// script or a folder of scripts in the configured PluginsPath, whose
// declarations — permissions, menus, logos — live statically in the
// scriptling metadata block under [tool.knot]. Nothing a plugin declares is
// produced by executing plugin code; loading is pure parsing, and plugin
// code only ever runs per-invocation in a user-bound environment.
//
// A folder plugin may also carry binary peers in bin/ (scriptling
// plugin-protocol executables). A peer is a component of the plugin, never a
// knot plugin by itself: it is spawned and handshaked at load so its
// declared name/version can verify the plugin's metadata requirements, but
// it registers nothing with knot.
package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/scriptling/metadata"
	"github.com/paularlott/scriptling/plugin"
)

// Menu is one declared sidebar entry. Permission, when set, is the fully
// qualified grant (plugin.<name>.<id>) that gates the item. Icon is a relative
// path to an SVG asset in the plugin folder; IconSVG holds its sanitized
// inner markup, rendered inline with the site's icon styling so a
// currentColor-stroked SVG themes like every built-in icon.
type Menu struct {
	PluginName string `json:"plugin,omitempty"`
	Label      string `json:"label"`
	URL        string `json:"url"`
	Permission string `json:"permission,omitempty"`
	Icon       string `json:"icon,omitempty"`
	IconSVG    string `json:"-"`
}

// Page is one declared page: an internal URL under /plugins/<name> whose
// handler runs per-request as the requesting user (the phase-2 dispatch
// model). Handler is a function in the entry file, or "module.fn" for a
// sibling module of a folder plugin.
// FieldHandler is a declared data source a template's custom fields can
// bind to: the space form turns the field into an autocompleter whose
// suggestions come from calling Handler with {_data: id, query: ...}.
type FieldHandler struct {
	Id      string // qualified: plugin.<name>.<id>
	Label   string
	Handler string // function in the entry file
	// Permission is an optional additive gate on the options endpoint:
	// UseSpaces (space forms drive the fetches) always applies, and a
	// declared gate narrows who may invoke the handler further.
	Permission string `json:"permission,omitempty"` // qualified grant
}

type Page struct {
	PluginName string `json:"plugin,omitempty"`
	Path       string `json:"path"`                 // "/dashboard" — served at /plugins/<name>/dashboard
	Handler    string `json:"handler"`              // function in the entry file ("module.fn" allowed)
	Label      string `json:"label,omitempty"`      // page title
	MenuLabel  string `json:"menu_label,omitempty"` // set: the page also appears in the sidebar under this label
	Permission string `json:"permission,omitempty"` // qualified grant, as Menu
	Icon       string `json:"icon,omitempty"`
	IconSVG    string `json:"-"`
	// Default marks the page as the post-login landing page. At most one
	// takes effect (first plugin by name, then declaration order) — see
	// DefaultPageURL.
	Default bool `json:"default,omitempty"`
}

// URL is the served location of the page.
func (pg Page) URL() string {
	return "/plugins/" + pg.PluginName + pg.Path
}

// Handler is a declared handler gate: one [[tool.knot.handlers]] entry. The
// declared permission is the handler's gate wherever it is called — same
// semantics as row/column gates — overriding the calling page's. A
// declaration is also the opt-in for plugin-root addressability
// (/plugins/<name>/<handler>): undeclared handlers inherit the calling
// page's gate and are only reachable through a page path.
type Handler struct {
	Handler    string `json:"handler"`              // function name or module.function
	Permission string `json:"permission,omitempty"` // qualified grant, as Page; empty: any logged-in user
}

// HandlerDecl returns the declared gate for a handler name, or nil when the
// handler has no [[tool.knot.handlers]] entry.
func (p *Plugin) HandlerDecl(name string) *Handler {
	for i := range p.Handlers {
		if p.Handlers[i].Handler == name {
			return &p.Handlers[i]
		}
	}
	return nil
}

// PermissionDecl is one declared permission, qualified to plugin.<name>.<id>.
type PermissionDecl struct {
	Id    string `json:"id"`    // fully qualified, e.g. plugin.metrics.read
	Label string `json:"label"` // derived display name, e.g. "read"
}

// Plugin is a successfully loaded plugin: its inert registration data.
type Plugin struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Dir         string `json:"-"`                    // the plugin folder
	EntryFile   string `json:"-"`                    // main.py or the single .py file
	LogoLight   string `json:"logo_light,omitempty"` // relative paths
	LogoDark    string `json:"logo_dark,omitempty"`
	// SiteLogo reports that the plugin's logo pair claims the main page
	// logo (login included) — a declared pair is the claim. At most one
	// plugin's claim takes effect — see SiteLogoURLs.
	SiteLogo      bool             `json:"site_logo,omitempty"`
	Permissions   []PermissionDecl `json:"permissions"`
	Menus         []Menu           `json:"menus"`
	Pages         []Page           `json:"pages"`
	Handlers      []Handler        `json:"handlers"`
	MCPTools      []MCPTool        `json:"mcp_tools"`
	FieldHandlers []FieldHandler   `json:"field_handlers"`
	Libs          []ScriptLib       `json:"libs,omitempty"`

	// EntrySource is the entry file's source, read once at load so request
	// dispatch does not touch the filesystem.
	EntrySource string `json:"-"`

	scope *plugin.Manager // owns this plugin's bin/ peers, nil when none
}

// Scope returns the manager owning this plugin's binary peers (which the
// handler environment registers as plugin.* libraries), or nil.
func (p *Plugin) Scope() *plugin.Manager { return p.scope }

// Peer reports one loaded binary peer's handshake identity.
type Peer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

// FailedPlugin records a plugin whose folder/file was present but which
// failed validation or requirements; its data (when storage exists) is
// retained and it is surfaced on the admin plugins page.
type FailedPlugin struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Registry is the in-memory result of one boot-time scan. Replace-on-boot,
// never mutated after Load returns.
type Registry struct {
	mu       sync.RWMutex
	plugins  []*Plugin
	failed   []FailedPlugin
	warnings []string
	manager  *plugin.Manager // parent for all plugin scopes
	closed   bool
}

var (
	globalRegistry *Registry
	globalMu       sync.RWMutex
)

// GetRegistry returns the boot-time registry, or nil when plugins are not
// configured / not yet loaded.
func GetRegistry() *Registry {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalRegistry
}

// SetRegistry replaces the global registry (tests; Load is the normal path).
func SetRegistry(r *Registry) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalRegistry = r
}

// Present reports whether the plugins path produced anything at all — loaded
// plugins, failed plugins, or load warnings. When false, the plugin surface
// is hidden entirely: no nav item, no admin page, no search entry.
func (r *Registry) Present() bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.plugins) > 0 || len(r.failed) > 0 || len(r.warnings) > 0
}

// All returns the successfully loaded plugins in name order.
func (r *Registry) All() []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Plugin, len(r.plugins))
	copy(out, r.plugins)
	return out
}

// ByName returns the loaded plugin with that name, or nil.
func (r *Registry) ByName(pluginName string) *Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if p.Name == pluginName {
			return p
		}
	}
	return nil
}

// Failed returns the plugins that were present but failed to load.
func (r *Registry) Failed() []FailedPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]FailedPlugin, len(r.failed))
	copy(out, r.failed)
	return out
}

// Warnings returns non-fatal load warnings (ignored files, unloadable
// binaries that no plugin declared a requirement on, …).
func (r *Registry) Warnings() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.warnings))
	copy(out, r.warnings)
	return out
}

// Peers reports the binary peers loaded for a plugin, with health.
func (r *Registry) Peers(p *Plugin) []Peer {
	r.mu.RLock()
	scope := p.scope
	r.mu.RUnlock()
	if scope == nil {
		return nil
	}
	var peers []Peer
	for _, md := range scope.List() {
		peer := Peer{Name: md.Name, Version: md.Version, Healthy: true}
		if health := scope.Health(); health != nil {
			if err, ok := health[md.Name]; ok && err != nil {
				peer.Healthy = false
				peer.Error = err.Error()
			}
		}
		peers = append(peers, peer)
	}
	return peers
}

// VisibleMenus returns the menu items the given user may see, gated per item
// exactly as the sidebar renders them: a declared permission requires one of
// the user's roles to carry the qualified plugin grant; a group requires
// membership. This is the single gate — the nav builder, the nav-preferences
// validation, and search all ask the same question, so "pinnable" and
// "visible" can never drift apart.
func (r *Registry) VisibleMenus(user *model.User) []Menu {
	if user == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var menus []Menu
	for _, p := range r.plugins {
		for _, menu := range p.Menus {
			if menu.Permission != "" && !user.HasPluginPermission(menu.Permission) {
				continue
			}
			menus = append(menus, menu)
		}
	}
	return menus
}

// VisibleMenuURLs is the set of URLs VisibleMenus yields, for membership
// checks (pin validation).
func (r *Registry) VisibleMenuURLs(user *model.User) map[string]bool {
	menus := r.VisibleMenus(user)
	if len(menus) == 0 {
		return nil
	}
	urls := make(map[string]bool, len(menus))
	for _, menu := range menus {
		urls[menu.URL] = true
	}
	return urls
}

// Page finds a loaded plugin and its declared page by URL path (the path
// after /plugins/<name>, e.g. "/dashboard"). Returns nils when unknown.
func (r *Registry) Page(pluginName, pagePath string) (*Plugin, *Page) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if p.Name != pluginName {
			continue
		}
		for i := range p.Pages {
			if p.Pages[i].Path == pagePath {
				return p, &p.Pages[i]
			}
		}
		return nil, nil
	}
	return nil, nil
}

// DefaultPageURL returns the URL of the page claimed as the post-login
// landing page, or "" when no plugin claims one. Plugins are name-sorted, so
// the first claimant by name wins deterministically on every node.
func (r *Registry) DefaultPageURL() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		for _, page := range p.Pages {
			if page.Default {
				return "/plugins/" + p.Name + page.Path
			}
		}
	}
	return ""
}

// SiteLogoURLs returns the served URLs of the plugin logos that replace the
// main page logo, or empty strings when no plugin declares a pair. All() is
// name-sorted, so the alphabetically first claim wins deterministically —
// every node scanning the same folder computes the same winner. An explicit
// server.ui.logo_url config beats any plugin (the caller applies that).
func (r *Registry) SiteLogoURLs() (light, dark string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if p.SiteLogo && p.LogoLight != "" {
			return "/plugins/" + p.Name + "/assets/" + p.LogoLight,
				"/plugins/" + p.Name + "/assets/" + p.LogoDark
		}
	}
	return "", ""
}

// MCPTool is one [[tool.knot.mcp_tools]] entry: a plugin handler exposed
// as an MCP tool. The optional permission/group narrows which users see
// and may call the tool (knot enforces at both list and execute time);
// both empty means any MCP user. No input schema is declared — the tool's
// parameters arrive in the handler's params dict and the MCP schema is an
// empty object.
type MCPTool struct {
	Name        string             `json:"name"`                // MCP tool name; defaults to the handler name
	Description string             `json:"description"`         // shown to MCP clients
	Handler     string             `json:"handler"`             // function in the entry file
	Permission  string             `json:"permission,omitempty"` // qualified grant, as Handler
	Parameters  []MCPToolParameter `json:"parameters,omitempty"` // optional: the tool's input schema
}

// MCPToolParameter is one declared tool parameter: what MCP clients (and
// LLMs) see in the tool's input schema. Parameters still arrive in the
// handler's params dict untyped — the declaration is documentation and
// client ergonomics, not coercion.
type MCPToolParameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`                  // string, int, float, bool, list
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// MCPToolParamSchemaType maps a declared parameter type to its JSON
// schema type; unknown types map to "string".
func MCPToolParamSchemaType(declared string) string {
	if t, ok := mcpToolParamTypes[declared]; ok && t != "" {
		return t
	}
	return "string"
}

// ScriptLib is a scriptling-authored library: a .py file in the plugin's
// libs/ folder, loaded in-process by knot's embedded scriptling (no CLI,
// no subprocess). Its public surface — functions, classes and constants
// not prefixed with "_" — becomes the plugin.<name> import in handler
// environments and user-created tools. Version comes from the optional
// [tool.knot.lib] table in the library's metadata block, and the consuming
// plugin's dependency declaration ("plugin.<lib> via <lib> >= x")
// validates against it the same way Go peer handshakes do.
type ScriptLib struct {
	Name    string `json:"name"`    // from the filename, e.g. libs/calc.py -> "calc"
	Version string `json:"version"` // [tool.knot.lib] version, default "1.0"
	Source  string `json:"-"`
}

// FieldHandlers lists every declared field handler across plugins.
func (r *Registry) FieldHandlers() []FieldHandler {
	var out []FieldHandler
	for _, plugin := range r.All() {
		out = append(out, plugin.FieldHandlers...)
	}
	return out
}

// FieldHandler looks a field handler up by its qualified id.
func (r *Registry) FieldHandler(id string) (*Plugin, *FieldHandler) {
	for _, plugin := range r.All() {
		for i := range plugin.FieldHandlers {
			if plugin.FieldHandlers[i].Id == id {
				return plugin, &plugin.FieldHandlers[i]
			}
		}
	}
	return nil, nil
}

// Close shuts down every spawned peer process. Called on server shutdown.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.manager == nil {
		r.closed = true
		return
	}
	r.manager.Close()
	r.closed = true
}

// pluginNameRe is the slug every plugin identity must match: it becomes part
// of the permission namespace (plugin.<name>.<id>).
var pluginNameRe = regexp.MustCompile(`^[a-z0-9_-]+$`)
var permissionIdRe = regexp.MustCompile(`^[a-z0-9_]+$`)

// QualifiedPermission builds the canonical grant for a plugin permission id.
func QualifiedPermission(pluginName, id string) string {
	return "plugin." + pluginName + "." + id
}

// peerLoadTimeout bounds the spawn+handshake of one binary peer so a broken
// binary cannot stall boot.
const peerLoadTimeout = 15 * time.Second

// scriptlingVersionForVerify reports the embedded scriptling runtime
// version a plugin's requires-scriptling is checked against. Variable so
// tests can pin one — test binaries carry no dependency versions, so the
// real function reports "unknown" under go test.
var scriptlingVersionForVerify = build.ScriptlingVersion

// Load scans the plugins path and builds the global registry. PluginsPath
// empty or missing disables plugins (nil registry, no error). Failures are
// per-plugin: a broken plugin is recorded and the server continues.
func Load(pluginsPath string) (*Registry, error) {
	logger := log.WithGroup("plugins")

	if pluginsPath == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(pluginsPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warn("plugins path does not exist, plugins disabled", "path", pluginsPath)
			return nil, nil
		}
		return nil, fmt.Errorf("plugins path: %w", err)
	}

	registry := &Registry{}
	candidates, warnings := scanCandidates(pluginsPath, entries)
	registry.warnings = append(registry.warnings, warnings...)

	var parent *plugin.Manager
	loaded := make([]*Plugin, 0, len(candidates))
	// MCP tool names share one namespace across every provider; plugins
	// are loaded in name order, so the first claimant wins deterministically
	// and a later duplicate is a load error.
	seenToolNames := map[string]string{}
	for _, c := range candidates {
		p, loadWarnings, err := loadPlugin(c, func() *plugin.Manager {
			if parent == nil {
				parent = plugin.NewManager(logger)
			}
			return parent
		})
		registry.warnings = append(registry.warnings, loadWarnings...)
		if err != nil {
			logger.Error("plugin failed to load", "plugin", c.name, "error", err)
			registry.failed = append(registry.failed, FailedPlugin{Name: c.name, Path: c.path, Reason: err.Error()})
			continue
		}
		dup := ""
		for _, tool := range p.MCPTools {
			if owner, taken := seenToolNames[tool.Name]; taken {
				dup = fmt.Sprintf("mcp tool %q already exposed by plugin %s", tool.Name, owner)
				break
			}
		}
		if dup != "" {
			logger.Error("plugin failed to load", "plugin", c.name, "error", dup)
			registry.failed = append(registry.failed, FailedPlugin{Name: c.name, Path: c.path, Reason: dup})
			continue
		}
		for _, tool := range p.MCPTools {
			seenToolNames[tool.Name] = p.Name
		}
		logger.Info("plugin loaded", "plugin", p.Name, "version", p.Version,
			"permissions", len(p.Permissions), "menus", len(p.Menus))
		loaded = append(loaded, p)
	}
	registry.manager = parent
	registry.plugins = loaded

	// Deterministic site-logo resolution with a warning when several plugins
	// claim it — the first by name wins, identically on every node.
	claimers := make([]string, 0, 1)
	for _, p := range loaded {
		if p.SiteLogo {
			claimers = append(claimers, p.Name)
		}
	}
	if len(claimers) > 1 {
		registry.warnings = append(registry.warnings, fmt.Sprintf(
			"site logo claimed by multiple plugins (%s); %s wins as the first by name; a declared logo pair is the claim, so only one plugin should ship one",
			strings.Join(claimers, ", "), claimers[0]))
	}

	// Same rule for the default (post-login) page claim.
	var defaultClaimers []string
	for _, p := range loaded {
		for _, page := range p.Pages {
			if page.Default {
				defaultClaimers = append(defaultClaimers, p.Name)
				break
			}
		}
	}
	if len(defaultClaimers) > 1 {
		registry.warnings = append(registry.warnings, fmt.Sprintf(
			"default page claimed by multiple plugins (%s); %s wins as the first by name",
			strings.Join(defaultClaimers, ", "), defaultClaimers[0]))
	}

	globalMu.Lock()
	old := globalRegistry
	globalRegistry = registry
	globalMu.Unlock()
	if old != nil {
		old.Close()
	}
	return registry, nil
}

// candidate is a discovered plugin location before metadata parsing.
type candidate struct {
	name string
	path string // the plugin folder's main.py
	dir  string // the plugin folder
}

// scanCandidates finds plugins: top-level .py files and folders containing
// main.py. A file and a folder may not both claim the same name.
func scanCandidates(root string, entries []os.DirEntry) ([]candidate, []string) {
	var warnings []string
	byName := make(map[string]candidate)

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		base := strings.TrimSuffix(name, ".py")

		if entry.IsDir() {
			mainPath := filepath.Join(root, name, "main.py")
			if _, err := os.Stat(mainPath); err != nil {
				warnings = append(warnings, fmt.Sprintf("folder %s has no main.py, ignored", name))
				continue
			}
			if !pluginNameRe.MatchString(name) {
				warnings = append(warnings, fmt.Sprintf("folder %s is not a valid plugin name ([a-z0-9_-]+), ignored", name))
				continue
			}
			if prev, ok := byName[name]; ok {
				warnings = append(warnings, fmt.Sprintf("plugin name %q claimed by both %s and folder %s; folder wins", name, prev.path, name))
			}
			byName[name] = candidate{name: name, path: mainPath, dir: filepath.Join(root, name)}
			continue
		}

		// Plugins are folders (main.py + assets + peers/ + bin/); a loose
		// .py is not a plugin.
		if filepath.Ext(name) == ".py" && pluginNameRe.MatchString(base) {
			warnings = append(warnings, fmt.Sprintf("file %s is not a plugin — plugins are folders with a main.py; ignored", name))
		}
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]candidate, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	return out, warnings
}

// loadPlugin parses and validates one candidate, loading its bin/ peers so
// metadata requirements can be verified. Returns the plugin, warnings, or an
// error that marks the plugin failed.
func loadPlugin(c candidate, newManager func() *plugin.Manager) (*Plugin, []string, error) {
	source, err := os.ReadFile(c.path)
	if err != nil {
		return nil, nil, fmt.Errorf("read entry file: %w", err)
	}

	m, ok, err := metadata.Parse(source)
	if err != nil {
		return nil, nil, fmt.Errorf("metadata block: %w", err)
	}
	var warnings []string

	knotTable, hasKnot := map[string]any{}, false
	if ok {
		if t, found := m.Tool("knot"); found {
			knotTable, hasKnot = t, true
		}
	}
	if !hasKnot {
		return nil, nil, fmt.Errorf("no [tool.knot] table in metadata block — a plugin must declare itself in its metadata")
	}

	p, err := parseToolKnot(c.name, c.dir, knotTable)
	if err != nil {
		return nil, nil, err
	}
	p.Dir = c.dir
	p.EntryFile = c.path
	p.EntrySource = string(source)
	for i := range p.Menus {
		p.Menus[i].PluginName = p.Name
	}
	for i := range p.Pages {
		p.Pages[i].PluginName = p.Name
	}

	// Load peers before verifying requirements so their declared versions
	// can satisfy them: bin/ hosts Go peers, peers/ hosts scriptling peers.
	scope, peerWarnings, err := loadPeers(c.name, c.dir, newManager)
	warnings = append(warnings, peerWarnings...)
	if err != nil {
		return nil, warnings, err
	}
	p.scope = scope

	if sp, err := loadScriptLibs(c.dir); err != nil {
		return nil, warnings, err
	} else {
		p.Libs = sp
	}

	if ok {
		// requires-scriptling bounds the embedded scriptling runtime the
		// plugin's code runs on — the language features it may use — so the
		// check runs against the embedded module's version, not knot's.
		// Dev builds (replace directive) and test binaries carry no
		// embedded version; the check is skipped there rather than failing
		// everything.
		hostVersion := scriptlingVersionForVerify()
		if hostVersion == "unknown" || hostVersion == "local" {
			m.RequiresScriptling = ""
		}
		env := metadata.Env{
			HostVersion: hostVersion,
			Resolves:    resolverFor(c, p),
			PluginVersion: func(name string) (string, bool) {
				for _, sp := range p.Libs {
					if sp.Name == name {
						return sp.Version, true
					}
				}
				if p.scope == nil {
					return "", false
				}
				for _, md := range p.scope.List() {
					if md.Name == name {
						return md.Version, true
					}
				}
				return "", false
			},
		}
		if err := m.Verify(env); err != nil {
			return nil, warnings, err
		}
	}

	return p, warnings, nil
}

// resolverFor answers metadata dependency resolution for a plugin: knot's
// embedded libraries, the libraries knot registers in plugin environments,
// and the plugin's own modules.
func resolverFor(c candidate, p *Plugin) func(string) bool {
	return func(name string) bool {
		if strings.HasPrefix(name, "knot.") {
			return true
		}
		if pluginEnvLibraries[name] {
			return true
		}

		// A module in the plugin's own folder ("helpers" → helpers.py).
		if !strings.Contains(name, ".") {
			if _, err := os.Stat(filepath.Join(c.dir, name+".py")); err == nil {
				return true
			}
			return false
		}
		rel := strings.ReplaceAll(name, ".", string(filepath.Separator))
		if _, err := os.Stat(filepath.Join(c.dir, rel+".py")); err == nil {
			return true
		}
		return false
	}
}

// loadScriptLibs reads the plugin's libs/ folder: one scriptling library
// per .py file, named by its filename. The optional [tool.knot.lib]
// table in the library's metadata block carries the version dependency
// declarations check; without it the version defaults to "1.0".
func loadScriptLibs(pluginDir string) ([]ScriptLib, error) {
	libsDir := filepath.Join(pluginDir, "libs")
	entries, err := os.ReadDir(libsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var libs []ScriptLib
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".py" || strings.HasPrefix(name, ".") {
			continue
		}
		peerName := strings.TrimSuffix(name, ".py")
		if !pluginNameRe.MatchString(peerName) {
			return nil, fmt.Errorf("libs/%s: not a valid library name ([a-z0-9_-]+)", name)
		}
		source, err := os.ReadFile(filepath.Join(libsDir, name))
		if err != nil {
			return nil, fmt.Errorf("libs/%s: %v", name, err)
		}
		version := "1.0"
		if m, ok, err := metadata.Parse(source); err != nil {
			return nil, fmt.Errorf("libs/%s: metadata block: %v", name, err)
		} else if ok {
			if t, found := m.Tool("knot.lib"); found {
				if v, ok := t["version"].(string); ok && v != "" {
					version = v
				}
			}
		}
		libs = append(libs, ScriptLib{Name: peerName, Version: version, Source: string(source)})
	}
	return libs, nil
}
