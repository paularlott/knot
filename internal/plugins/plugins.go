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
// qualified grant (plugin.<name>.<id>) that gates the item; Group, when set,
// additionally restricts it to members of that group. Icon is a relative
// path to an SVG asset in the plugin folder; IconSVG holds its sanitized
// inner markup, rendered inline with the site's icon styling so a
// currentColor-stroked SVG themes like every built-in icon.
type Menu struct {
	PluginName string `json:"plugin,omitempty"`
	Label      string `json:"label"`
	URL        string `json:"url"`
	Permission string `json:"permission,omitempty"`
	Group      string `json:"group,omitempty"`
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
	// Permission and Group are optional additive gates on the options
	// endpoint: UseSpaces (space forms drive the fetches) always applies,
	// and a declared gate narrows who may invoke the handler further.
	Permission string `json:"permission,omitempty"` // qualified grant
	Group      string `json:"group,omitempty"`
}

type Page struct {
	PluginName string `json:"plugin,omitempty"`
	Path       string `json:"path"`                 // "/dashboard" — served at /plugins/<name>/dashboard
	Handler    string `json:"handler"`              // function in the entry file ("module.fn" allowed)
	Label      string `json:"label,omitempty"`      // page title
	MenuLabel  string `json:"menu_label,omitempty"` // set: the page also appears in the sidebar under this label
	Permission string `json:"permission,omitempty"` // qualified grant, as Menu
	Group      string `json:"group,omitempty"`
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
// declared permission/group is the handler's gate wherever it is called —
// same semantics as row/column gates — overriding the calling page's. A
// declaration is also the opt-in for plugin-root addressability
// (/plugins/<name>/<handler>): undeclared handlers inherit the calling
// page's gate and are only reachable through a page path.
type Handler struct {
	Handler    string `json:"handler"`              // function name or module.function
	Permission string `json:"permission,omitempty"` // qualified grant, as Page
	Group      string `json:"group,omitempty"`      // both empty: any logged-in user
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
	Dir         string `json:"-"`                    // plugin folder (single-file: parent dir)
	Folder      bool   `json:"-"`                    // folder plugin (vs single file)
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
	FieldHandlers []FieldHandler   `json:"field_handlers"`

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
			if menu.Group != "" {
				groups := []string{menu.Group}
				if !user.HasAnyGroup(&groups) {
					continue
				}
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
func (r *Registry) SiteLogoURLs() (light, dark, pluginName string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if p.SiteLogo && p.LogoLight != "" {
			return "/plugins/" + p.Name + "/assets/" + p.LogoLight,
				"/plugins/" + p.Name + "/assets/" + p.LogoDark,
				p.Name
		}
	}
	return "", "", ""
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
	name       string
	path       string // entry file path
	dir        string // plugin folder ("" for single-file: parent of file)
	singleFile bool
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

		if filepath.Ext(name) != ".py" {
			continue
		}
		if !pluginNameRe.MatchString(base) {
			warnings = append(warnings, fmt.Sprintf("file %s is not a valid plugin name ([a-z0-9_-]+), ignored", name))
			continue
		}
		if _, ok := byName[base]; ok {
			warnings = append(warnings, fmt.Sprintf("plugin name %q claimed by both a folder and file %s; folder wins", base, name))
			continue
		}
		byName[base] = candidate{name: base, path: filepath.Join(root, name), dir: root, singleFile: true}
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

	p, err := parseToolKnot(c.name, c.dir, c.singleFile, knotTable)
	if err != nil {
		return nil, nil, err
	}
	p.Dir = c.dir
	p.Folder = !c.singleFile
	p.EntryFile = c.path
	p.EntrySource = string(source)
	for i := range p.Menus {
		p.Menus[i].PluginName = p.Name
	}
	for i := range p.Pages {
		p.Pages[i].PluginName = p.Name
	}

	// Load bin/ peers (folder plugins only) before verifying requirements so
	// the peers' declared versions can satisfy them.
	if !c.singleFile {
		scope, peerWarnings, err := loadPeers(c.name, c.dir, newManager)
		warnings = append(warnings, peerWarnings...)
		if err != nil {
			return nil, warnings, err
		}
		p.scope = scope
	}

	if ok {
		env := metadata.Env{
			HostVersion: build.Version,
			Resolves:    resolverFor(c, p),
			PluginVersion: func(name string) (string, bool) {
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
		if c.singleFile {
			return false
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
