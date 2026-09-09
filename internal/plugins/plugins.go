// Package plugins implements knot's plugin system: a plugin is a folder in
// the configured PluginsPath whose declarations — permissions, menus, pages,
// handlers, MCP tools, logos, icons — live statically in a [tool.knot]
// table. Nothing a plugin declares is produced by executing plugin code;
// loading is pure parsing, and plugin code only ever runs per-invocation in
// a user-bound environment.
//
// The [tool.knot] table is sourced two ways, parsed identically:
//
//   - Pure-script plugin: knot reads main.py and parses its metadata block.
//   - Peer plugin: a bin/ peer (a scriptling plugin-protocol executable in
//     any language) returns the table as static manifest data in its
//     handshake (Custom["tool.knot"]); knot reads it there. Such a plugin
//     needs no companion main.py. A peer plugin may still ship a main.py for
//     scriptling handlers, in which case main.py's block is authoritative.
//
// Handlers a plugin declares are addressed as plugin.<name>.<function>: a
// peer exports them over the plugin protocol, a main.py exposes them by
// being registered as the plugin.<name> library — the dispatch layer calls
// the qualified name and does not care which kind answers.
//
// A peer is a component of the plugin: it is spawned and handshaked at load
// so its manifest and version are available for declaration parsing and
// requirement verification. Whether it also registers a callable surface
// depends on the plugin's handler declarations.
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
// handler runs per-request as the requesting user. Handler is a function in the entry file, or "module.fn" for a
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
	// APIVersion is the plugin system generation this plugin targets
	// ([tool.knot] api; absent = 1, the only generation today). Breaking
	// changes to the plugin contract — the [tool.knot] shape, the handler
	// request/response contract, the block document — happen only behind a
	// new generation, and a knot that doesn't know a generation rejects the
	// plugin loudly at load rather than mis-parsing it.
	APIVersion int    `json:"api,omitempty"`
	Dir        string `json:"-"`                    // the plugin folder
	EntryFile  string `json:"-"`                    // main.py
	LogoLight  string `json:"logo_light,omitempty"` // relative paths
	LogoDark   string `json:"logo_dark,omitempty"`
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
	Libs          []ScriptLib      `json:"libs,omitempty"`

	// ActionIcons maps declared icon asset paths to their sanitized inner
	// SVG markup ([tool.knot] icons). Menus and pages carry their icons
	// statically; row actions are data-driven, so their icons are declared
	// once here, sanitized at load like every other plugin asset, and
	// addressable by path from a handler's action JSON at runtime.
	ActionIcons map[string]string `json:"icons,omitempty"`

	// EntrySource is the entry file's source, read once at load so request
	// dispatch does not touch the filesystem. Empty for a peer plugin with
	// no main.py.
	EntrySource string `json:"-"`

	// ScriptNamespace is the plugin.<x> library name a pure-script plugin's
	// main.py is registered and addressed under: the folder name sanitized
	// to a scriptling identifier (ScriptNamespace). Empty for a peer plugin
	// with no main.py. This is the "<name>" a bare handler declaration
	// (handler = "status") resolves against.
	ScriptNamespace string `json:"-"`

	// Namespaces are the plugin.<x> library names this plugin's declared
	// handlers resolve under, in the order they must be imported to
	// materialize the plugin.<x> bindings for dispatch. A pure-script
	// plugin contributes its folder name (its main.py registers as
	// plugin.<folder>); a peer plugin contributes each peer's handshake
	// name (the peer registers plugin.<peer>). A plugin with both
	// contributes both.
	Namespaces []string `json:"-"`

	scope *plugin.Manager // owns this plugin's bin/ peers, nil when none
}

// Scope returns the manager owning this plugin's binary peers (which the
// handler environment registers as plugin.* libraries), or nil.
func (p *Plugin) Scope() *plugin.Manager { return p.scope }

// DefaultNamespace is the plugin.<ns> a bare handler declaration resolves
// against: the ScriptNamespace when the plugin has a main.py (its handlers
// live there), otherwise its first peer namespace (a peer plugin's handlers
// are the peer's exports). Empty only for a plugin with neither, which
// cannot declare a callable handler. A manifest may still name a specific
// peer explicitly as plugin.<peer>.<fn> to disambiguate multiple peers.
func (p *Plugin) DefaultNamespace() string {
	if p.ScriptNamespace != "" {
		return p.ScriptNamespace
	}
	if len(p.Namespaces) > 0 {
		return p.Namespaces[0]
	}
	return ""
}

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
			if !user.PassesPluginGate(menu.Permission) {
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
// as an MCP tool. The optional permission narrows which users see
// and may call the tool (knot enforces at both list and execute time);
// both empty means any MCP user. No input schema is declared — the tool's
// parameters arrive in the handler's params dict and the MCP schema is an
// empty object.
type MCPTool struct {
	Name        string             `json:"name"`                 // MCP tool name; defaults to the handler name
	Description string             `json:"description"`          // shown to MCP clients
	Handler     string             `json:"handler"`              // function in the entry file
	Permission  string             `json:"permission,omitempty"` // qualified grant, as Handler
	Parameters  []MCPToolParameter `json:"parameters,omitempty"` // optional: the tool's input schema
}

// MCPToolParameter is one declared tool parameter: what MCP clients (and
// LLMs) see in the tool's input schema. Parameters still arrive in the
// handler's params dict untyped — the declaration is documentation and
// client ergonomics, not coercion.
type MCPToolParameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string, int, float, bool, list
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

// ScriptNamespace maps a plugin folder name to the scriptling module
// identifier its main.py is registered under (plugin.<ScriptNamespace>).
// Folder names may contain "-" (a legal plugin name) but module names may
// not, so hyphens become underscores; already-valid names are unchanged.
// Deterministic, so every node computes the same import name.
func ScriptNamespace(pluginName string) string {
	return strings.ReplaceAll(pluginName, "-", "_")
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

// candidate is a discovered plugin location before metadata parsing. path is
// the folder's main.py when it has one (a pure-script plugin, or a peer
// plugin that also ships scriptling handlers); empty for a peer plugin whose
// declarations come from its bin/ peer's handshake manifest.
type candidate struct {
	name string
	path string // the plugin folder's main.py, or "" for a manifest-in-peer plugin
	dir  string // the plugin folder
}

// scanCandidates finds plugins: folders that either contain a main.py
// (pure-script, or a peer plugin that also ships scriptling handlers) or a
// bin/ directory (a peer plugin whose declarations come from the peer's
// handshake manifest — no companion main.py needed). A loose top-level .py
// is not a plugin.
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
			dir := filepath.Join(root, name)
			mainPath := filepath.Join(dir, "main.py")
			hasMain := statIsFile(mainPath)
			hasBin := statIsDir(filepath.Join(dir, "bin"))
			if !hasMain && !hasBin {
				warnings = append(warnings, fmt.Sprintf("folder %s has no main.py and no bin/ peers, ignored", name))
				continue
			}
			if !pluginNameRe.MatchString(name) {
				warnings = append(warnings, fmt.Sprintf("folder %s is not a valid plugin name ([a-z0-9_-]+), ignored", name))
				continue
			}
			if prev, ok := byName[name]; ok {
				warnings = append(warnings, fmt.Sprintf("plugin name %q claimed by both %s and folder %s; folder wins", name, prev.path, name))
			}
			// A folder with main.py sources its manifest from that file even
			// when it also carries bin/ peers; a bin/-only folder sources it
			// from the peer handshake (path left empty).
			path := ""
			if hasMain {
				path = mainPath
			}
			byName[name] = candidate{name: name, path: path, dir: dir}
			continue
		}

		// Plugins are folders (main.py + assets + libs/ + bin/); a loose
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

// manifestKnotKey is the key under a peer's handshake Custom metadata that
// carries its [tool.knot] declaration table. A peer sets it once, verbatim
// (scriptling Server.SetMetadata); knot reads it and parses it with the same
// parseToolKnot a pure-script plugin's block goes through. Serving it is the
// peer's protocol layer answering the handshake — no handler runs to produce
// it, so cluster determinism holds.
const manifestKnotKey = "tool.knot"

// statIsFile reports whether path exists and is a regular file.
func statIsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// statIsDir reports whether path exists and is a directory.
func statIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// loadPlugin parses and validates one candidate, loading its bin/ peers so
// metadata requirements can be verified and — for a peer plugin without a
// main.py — so its handshake manifest can be read. The [tool.knot]
// declaration table is sourced from main.py when present, else from the
// peer's handshake Custom["tool.knot"]. Returns the plugin, warnings, or an
// error that marks the plugin failed.
func loadPlugin(c candidate, newManager func() *plugin.Manager) (*Plugin, []string, error) {
	var warnings []string

	// A main.py-backed plugin parses its metadata block for both the
	// [tool.knot] table and the requires-scriptling / dependency
	// verification. A bin/-only peer plugin has neither here — its manifest
	// arrives from the peer handshake below, and requirement verification is
	// the peer's own concern.
	var m *metadata.Metadata
	var source []byte
	if c.path != "" {
		var err error
		source, err = os.ReadFile(c.path)
		if err != nil {
			return nil, nil, fmt.Errorf("read entry file: %w", err)
		}
		parsed, ok, err := metadata.Parse(source)
		if err != nil {
			return nil, nil, fmt.Errorf("metadata block: %w", err)
		}
		if ok {
			m = &parsed
		}
	}

	// Load peers before parsing the manifest and verifying requirements: a
	// peer plugin's declarations live in the peer's handshake, and a
	// main.py plugin's declared peer versions must satisfy its metadata.
	scope, peerWarnings, err := loadPeers(c.name, c.dir, newManager)
	warnings = append(warnings, peerWarnings...)
	if err != nil {
		return nil, warnings, err
	}

	// Resolve the [tool.knot] declaration table: main.py wins when present,
	// else the peer handshake manifest.
	knotTable, source, err := resolveKnotTable(c, m, source, scope)
	if err != nil {
		return nil, warnings, err
	}

	p, err := parseToolKnot(c.name, c.dir, knotTable)
	if err != nil {
		return nil, warnings, err
	}
	p.Dir = c.dir
	p.EntryFile = c.path
	p.EntrySource = string(source)
	p.scope = scope
	for i := range p.Menus {
		p.Menus[i].PluginName = p.Name
	}
	for i := range p.Pages {
		p.Pages[i].PluginName = p.Name
	}

	if sp, err := loadScriptLibs(c.dir); err != nil {
		return nil, warnings, err
	} else {
		p.Libs = sp
	}

	// The plugin.<x> namespaces this plugin's declared handlers resolve
	// under, imported at dispatch to materialize the bindings: a
	// scriptling-identifier form of the folder name when main.py backs
	// handlers (folder names may contain "-", which is not a legal module
	// name, so it is sanitized to "_"), plus each peer's handshake name
	// (already a valid identifier). ScriptNamespace is the one main.py
	// registers under; peer namespaces are their handshake names verbatim.
	if p.EntrySource != "" {
		p.ScriptNamespace = ScriptNamespace(p.Name)
		p.Namespaces = append(p.Namespaces, p.ScriptNamespace)
	}
	if scope != nil {
		for _, md := range scope.List() {
			// A peer's handshake name is namespaced under "plugin." by
			// scriptling (NamespacePrefix); knot stores the bare namespace
			// and adds the prefix itself at import/dispatch, so peer and
			// main.py namespaces are handled uniformly.
			p.Namespaces = append(p.Namespaces, strings.TrimPrefix(md.Name, "plugin."))
		}
	}

	if m != nil {
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

// resolveKnotTable returns the [tool.knot] declaration table for a candidate
// and the entry source to record. When main.py is present its metadata block
// is authoritative (source is its bytes). Otherwise the table comes from a
// bin/ peer's handshake Custom["tool.knot"] — a peer plugin declares itself
// once, in the peer, with no companion main.py — and the entry source is
// empty. A peer plugin with several peers: the peer whose handshake name
// matches the plugin folder wins, else the sole peer, else it is an error
// (ambiguous — knot cannot pick a manifest).
func resolveKnotTable(c candidate, m *metadata.Metadata, source []byte, scope *plugin.Manager) (map[string]any, []byte, error) {
	if m != nil {
		if t, found := m.Tool("knot"); found {
			return t, source, nil
		}
		return nil, source, fmt.Errorf("no [tool.knot] table in metadata block — a plugin must declare itself in its metadata")
	}

	// No main.py: the manifest must come from a peer handshake.
	if scope == nil {
		return nil, nil, fmt.Errorf("plugin has neither a main.py nor a loadable bin/ peer to declare itself")
	}
	list := scope.List()
	if len(list) == 0 {
		return nil, nil, fmt.Errorf("plugin has no main.py and no peer handshaked to supply a manifest")
	}

	var chosen *plugin.Metadata
	for i := range list {
		if list[i].Name == c.name {
			chosen = &list[i]
			break
		}
	}
	if chosen == nil {
		if len(list) == 1 {
			chosen = &list[0]
		} else {
			return nil, nil, fmt.Errorf("plugin has no main.py and multiple peers, none named %q — cannot pick which peer's manifest declares the plugin", c.name)
		}
	}

	table, err := knotTableFromCustom(chosen.Custom)
	if err != nil {
		return nil, nil, fmt.Errorf("peer %q: %w", chosen.Name, err)
	}
	return table, nil, nil
}

// knotTableFromCustom extracts the [tool.knot] declaration table from a
// peer's handshake Custom metadata. The peer sets Custom["tool.knot"] to the
// table (scriptling Server.SetMetadata); knot reads it verbatim. JSON
// transport decodes nested tables as map[string]any, which is exactly what
// parseToolKnot consumes.
func knotTableFromCustom(custom map[string]any) (map[string]any, error) {
	if custom == nil {
		return nil, fmt.Errorf("handshake carries no custom manifest data — the peer must declare its [tool.knot] block via SetMetadata")
	}
	raw, ok := custom[manifestKnotKey]
	if !ok {
		return nil, fmt.Errorf("handshake custom manifest has no %q key — the peer must declare its [tool.knot] block", manifestKnotKey)
	}
	table, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("handshake custom %q must be a table, got %T", manifestKnotKey, raw)
	}
	return table, nil
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
		libName := strings.TrimSuffix(name, ".py")
		if !pluginNameRe.MatchString(libName) {
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
		libs = append(libs, ScriptLib{Name: libName, Version: version, Source: string(source)})
	}
	return libs, nil
}
