package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/dns"
	"github.com/paularlott/knot/internal/plugins"
	knotscriptling "github.com/paularlott/knot/internal/scriptling"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/logger"
	ai "github.com/paularlott/mcp/ai"
	mcpopenai "github.com/paularlott/mcp/ai/openai"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/extlibs"
	"github.com/paularlott/scriptling/extlibs/agent"
	scriptlingai "github.com/paularlott/scriptling/extlibs/ai"
	aimemory "github.com/paularlott/scriptling/extlibs/ai/memory"
	scriptlingaitools "github.com/paularlott/scriptling/extlibs/ai/tools"
	scriptlingmcp "github.com/paularlott/scriptling/extlibs/mcp"
	"github.com/paularlott/scriptling/extlibs/messaging/discord"
	"github.com/paularlott/scriptling/extlibs/messaging/slack"
	"github.com/paularlott/scriptling/extlibs/messaging/telegram"
	"github.com/paularlott/scriptling/extlibs/netsecurity"
	provisionfetch "github.com/paularlott/scriptling/extlibs/provision/fetch"
	provisionfile "github.com/paularlott/scriptling/extlibs/provision/file"
	scriptlingsimilarity "github.com/paularlott/scriptling/extlibs/similarity"
	"github.com/paularlott/scriptling/libloader"
	"github.com/paularlott/scriptling/object"
	pluginpkg "github.com/paularlott/scriptling/plugin"
	"github.com/paularlott/scriptling/stdlib"
)

var (
	libraryFetcher func(string) (string, error)
)

// registerBaseLibraries registers common libraries shared across all environments
// customLogger is optional - pass nil to use the default logger
// scriptNetConfig shares knot's configured nameservers with scriptling's
// dial paths (requests, wait_for, websocket) via a resolver-only network
// configuration — no address checks, full access. Nil when no nameservers
// are configured (e.g. on the agent, where the container's system resolver
// already forwards through the resident resolver), leaving the system
// resolver in place.
func scriptNetConfig() *netsecurity.Config {
	if ns := dns.GetDefaultResolver().Nameservers(); len(ns) > 0 {
		return &netsecurity.Config{AllowAll: true, DNSServers: ns}
	}
	return nil
}

func registerBaseLibraries(env *scriptling.Scriptling, customLogger logger.Logger) {
	netPolicy := scriptNetConfig()
	stdlib.RegisterAll(env)
	extlibs.RegisterRequestsLibrary(env, netPolicy)
	extlibs.RegisterSecretsLibrary(env)
	extlibs.RegisterHTMLParserLibrary(env)
	extlibs.RegisterWaitForLibrary(env, netPolicy)
	extlibs.RegisterYAMLLibrary(env)
	if customLogger != nil {
		extlibs.RegisterLoggingLibrary(env, customLogger)
	} else {
		extlibs.RegisterLoggingLibraryDefault(env)
	}

	scriptlingai.Register(env)
	agent.Register(env)
	scriptlingaitools.Register(env)
	scriptlingsimilarity.Register(env)
	scriptlingmcp.Register(env)
	scriptlingmcp.RegisterToon(env)
	scriptlingmcp.RegisterToolHelpers(env)

	// Outbound messaging sends (telegram, discord, slack) — safe server-side.
	telegram.Register(env, nil)
	discord.Register(env, nil)
	slack.Register(env, nil)

	extlibs.RegisterTOMLLibrary(env)
	extlibs.RegisterTemplateHTMLLibrary(env)
	extlibs.RegisterTemplateTextLibrary(env)
	extlibs.RegisterShlexLibrary(env) // shlex — pure string processing, no filesystem access
	extlibs.RegisterCsvLibrary(env)   // scriptling.csv — CSV parsing/formatting, no filesystem access
	extlibs.RegisterXmlLibrary(env)   // scriptling.xml — XML parsing/formatting, no filesystem access
}

// registerServerFSPaths registers the fs library in server-side environments
// (MCP tools, event sinks) only when the admin has configured an allow-list of
// paths. Unregistered by default — server-side scripts get no local filesystem
// access. An empty (but non-nil) allow-list would deny everything, so it is
// treated as "not configured" and the library is skipped either way.
func registerServerFSPaths(env *scriptling.Scriptling) {
	if cfg := config.GetServerConfig(); cfg != nil && len(cfg.ScriptFSAllowedPaths) > 0 {
		extlibs.RegisterFSLibrary(env, cfg.ScriptFSAllowedPaths)
	}
}

// registerKnotLibraries registers all Knot-specific libraries for scriptling environments.
// If mcpLib is provided, it will be registered instead of creating a new one via GetMCPToolsLibrary.
// aiClient may be nil for local/remote environments where no AI client is available.
// withMethods controls whether knot.methods / knot.methods.schema are registered —
// false for server-side envs (MCP tool execution, event sinks) where method
// registration is not possible; true for all agent-side envs.
func registerKnotLibraries(env *scriptling.Scriptling, client *apiclient.ApiClient, userId string, mcpParams map[string]object.Object, mcpLib *knotscriptling.MCPLibrary, aiClient ai.Client, withMethods bool) {
	// knot.ai is always registered - Client() will return error if aiClient is nil
	env.RegisterLibrary(knotscriptling.GetAILibrary(aiClient))

	// knot.methods and knot.methods.schema are registered in agent/CLI envs
	// only, not MCP tool execution envs. The methodsRegistrar global gates
	// whether register() actually succeeds.
	if withMethods {
		env.RegisterLibrary(knotscriptling.GetMethodsLibrary())
		env.RegisterLibrary(knotscriptling.GetMethodsSchemaLibrary())
	}

	if client != nil {
		// Go transport layer - Python libs (knot.space, knot.user, etc.) resolve via import
		env.RegisterLibrary(knotscriptling.GetApiClientLibrary(client.GetRESTClient(), userId))

		if mcpLib != nil {
			env.RegisterLibrary(mcpLib.GetLibrary())
		} else {
			env.RegisterLibrary(knotscriptling.GetMCPToolsLibrary(client, mcpParams))
		}
	}
}

// registerAgentLibraries registers the complete library surface for every
// agent-side environment (space scripts, run-script, streaming, health checks,
// methods registration). It mirrors the scriptling CLI's setup.Scriptling
// (v0.21.x) but is knot's own explicit list: container and nomad are
// deliberately absent — scripts in a space manage containers through
// knot.space etc., never the runtime directly — and new scriptling libraries
// only appear here when knot opts in. network libs use knot's resolver
// configuration (scriptNetConfig). A nil log gets the default logging library
// and a null logger for the libs that need one.
func registerAgentLibraries(env *scriptling.Scriptling, log logger.Logger) {
	netPolicy := scriptNetConfig()
	stdlib.RegisterAll(env)

	aux := log
	if aux == nil {
		aux = logger.NewNullLogger()
	}

	extlibs.RegisterYAMLLibrary(env)
	extlibs.RegisterTOMLLibrary(env)
	extlibs.RegisterHTMLParserLibrary(env)
	extlibs.RegisterRequestsLibrary(env, netPolicy)
	extlibs.RegisterSecretsLibrary(env)
	extlibs.RegisterOSLibrary(env, nil)
	extlibs.RegisterLoggingLibrary(env, log)
	extlibs.RegisterSubprocessLibrary(env)
	extlibs.RegisterPathlibLibrary(env, nil)
	extlibs.RegisterGlobLibrary(env, nil)
	extlibs.RegisterTempfileLibrary(env, nil)
	extlibs.RegisterShutilLibrary(env, nil)
	extlibs.RegisterShlexLibrary(env)
	extlibs.RegisterZipfileLibrary(env, nil)
	extlibs.RegisterTarfileLibrary(env, nil)
	extlibs.RegisterCsvLibrary(env)
	extlibs.RegisterXmlLibrary(env)
	extlibs.RegisterFSLibrary(env, nil)
	extlibs.RegisterGrepLibrary(env, nil)
	extlibs.RegisterFindLibrary(env, nil)
	extlibs.RegisterSedLibrary(env, nil)
	extlibs.RegisterWaitForLibrary(env, netPolicy)
	extlibs.RegisterTemplateHTMLLibrary(env)
	extlibs.RegisterTemplateTextLibrary(env)

	provisionfile.Register(env)
	provisionfetch.Register(env)

	scriptlingai.Register(env)
	aimemory.Register(env, aux)
	agent.Register(env)
	scriptlingaitools.Register(env) // scriptling.ai.tools — knot registers this in every env
	scriptlingsimilarity.Register(env)

	telegram.Register(env, aux)
	discord.Register(env, aux)
	slack.Register(env, aux)

	scriptlingmcp.Register(env)
	scriptlingmcp.RegisterToon(env)
	scriptlingmcp.RegisterToolHelpers(env)

	// Agent-side scriptling plugins (--plugin / --plugin-dir), loaded once at
	// agent startup. This is how the agent reaches heavyweight drivers (e.g.
	// the scriptling database plugins) without compiling them into the agent
	// binary. nil when none are configured.
	if mgr := getAgentPluginManager(); mgr != nil {
		pluginpkg.RegisterLibraries(env, mgr)
	}
}

// newServerLibraryLoader creates a FuncLoader that fetches libraries from the server API
func newServerLibraryLoader(client *apiclient.ApiClient) libloader.LibraryLoader {
	return libloader.NewFuncLoader(func(name string) (string, bool, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		content, err := client.GetScriptLibrary(ctx, name)
		if err != nil {
			return "", false, nil // Not found or error
		}
		return content, true, nil
	}, "server-api")
}

// newServerLibraryLoaderWithContext creates a FuncLoader that fetches libraries from the server API with user context
func newServerLibraryLoaderWithContext(client *apiclient.ApiClient, user *model.User) libloader.LibraryLoader {
	return libloader.NewFuncLoader(func(name string) (string, bool, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ctx = context.WithValue(ctx, "user", user)
		content, err := client.GetScriptLibrary(ctx, name)
		if err != nil {
			return "", false, nil // Not found or error
		}
		return content, true, nil
	}, "server-api-with-user")
}

// newFetcherLoader creates a FuncLoader that uses the global libraryFetcher
func newFetcherLoader() libloader.LibraryLoader {
	return libloader.NewFuncLoader(func(name string) (string, bool, error) {
		if libraryFetcher == nil {
			return "", false, nil
		}
		content, err := libraryFetcher(name)
		if err != nil {
			return "", false, nil
		}
		return content, true, nil
	}, "fetcher")
}

// newKnotLibsLoader returns a loader for the embedded knot Python libs,
// with optional disk override via KnotLibPath config for development.
// KnotLibPath should point to the directory containing the knot/ subfolder
// e.g. internal/scriptling/lib/
func newKnotLibsLoader() libloader.LibraryLoader {
	cfg := config.GetServerConfig()
	if cfg != nil && cfg.KnotLibPath != "" {
		return libloader.NewFilesystem(cfg.KnotLibPath)
	}
	// Load from embedded FS: map "knot.space" -> "lib/knot/space.py"
	return libloader.NewFuncLoader(func(name string) (string, bool, error) {
		if !strings.HasPrefix(name, "knot.") {
			return "", false, nil
		}
		fileName := "lib/knot/" + strings.TrimPrefix(name, "knot.") + ".py"
		data, err := knotscriptling.EmbeddedLibs.ReadFile(fileName)
		if err != nil {
			return "", false, nil
		}
		return string(data), true, nil
	}, "knot-embedded-libs")
}

// setupLibraryLoader sets up library loading from configured libdir and/or server
func setupLibraryLoader(env *scriptling.Scriptling, client *apiclient.ApiClient) {
	var loaders []libloader.LibraryLoader

	// Knot Python libs first (embedded or disk override)
	loaders = append(loaders, newKnotLibsLoader())

	// Add filesystem loader if libdir is configured
	cfg := config.GetServerConfig()
	if cfg != nil && cfg.LibDir != "" {
		loaders = append(loaders, libloader.NewFilesystem(cfg.LibDir))
	}

	// Add server API loader or fetcher loader
	if client != nil {
		loaders = append(loaders, newServerLibraryLoader(client))
	} else if libraryFetcher != nil {
		loaders = append(loaders, newFetcherLoader())
	}

	env.SetLibraryLoader(libloader.NewChain(loaders...))
}

// muxHTTPPool wraps an *http.Client to implement pool.HTTPPool
type muxHTTPPool struct {
	httpClient *http.Client
}

func (p *muxHTTPPool) GetHTTPClient() *http.Client {
	return p.httpClient
}

// createServerAIClient creates an AI client that connects to the server's
// OpenAI-compatible endpoint. The server handles all tool discovery, execution,
// and per-user scoping via the MCPServerContext middleware. The endpoint only
// injects the default model if none is specified, and only adds a system prompt
// if no system message exists.
// For MuxClient (base URL is empty), requests are routed through the API mux
// directly with the user injected into context, bypassing real HTTP and auth.
// Returns nil if client is nil or creation fails.
func createServerAIClient(client *apiclient.ApiClient, user *model.User) ai.Client {
	if client == nil {
		return nil
	}

	baseURL := client.GetBaseURL()
	if baseURL == "" {
		// MuxClient: route through the API mux directly
		if user == nil {
			return nil
		}
		serverClient, err := mcpopenai.New(mcpopenai.Config{
			BaseURL:        "http://localhost/v1/",
			HTTPPool:       &muxHTTPPool{httpClient: rest.NewMuxHTTPClient(user)},
			RequestTimeout: 0,
		})
		if err != nil {
			return nil
		}
		return serverClient
	}

	// Real HTTP client: use base URL and auth token
	baseURL = strings.TrimRight(baseURL, "/") + "/v1/"
	serverClient, err := mcpopenai.New(mcpopenai.Config{
		BaseURL:        baseURL,
		APIKey:         client.GetAuthToken(),
		RequestTimeout: 0,
	})
	if err != nil {
		return nil
	}
	return serverClient
}

// ServerScriptlingOptions configures a server-side scriptling environment.
// Exactly one mode is selected by what is set: MCPParams (with User) selects
// MCP tool execution — result capture via the returned MCPLibrary; EventEnvelope
// (with User) selects event sink execution — knot.event accessors and the event
// payload/metadata variables.
type ServerScriptlingOptions struct {
	User          *model.User
	MCPParams     map[string]object.Object
	EventParams   map[string]object.Object
	EventEnvelope *EventEnvelope
}

// NewServerScriptlingEnv creates the scriptling environment for scripts that
// run in the knot server: MCP tool execution and event sink scripts. This is
// the restricted side of knot's two environments — no system access libraries
// (subprocess, os, pathlib, …), no runtime, fs only when the admin configured
// ScriptFSAllowedPaths, no knot.methods, and an HTTP-only plugin scope (no
// local executables on the server). Output is captured and returned.
//
// Both modes get: stdlib + the base extended libraries, knot.ai, the
// knot.apiclient transport, knot.mcp, all knot.* API libraries via the
// embedded loader, on-demand user `lib` scripts from the server, and a
// per-execution isolated plugin scope (released via the returned cleanup).
// MCP tool mode additionally gets result capture (mcp.return_*); event sink
// mode additionally gets the knot.event sink accessors (no emit — prevents
// sink → event → sink recursion) and the event payload/metadata variables.
//
// With a nil client the environment is base-only (used by tests); knot
// libraries and the loader require a client and user.
func NewServerScriptlingEnv(client *apiclient.ApiClient, opts ServerScriptlingOptions) (*scriptling.Scriptling, *knotscriptling.MCPLibrary, func(), error) {
	env := scriptling.New()
	env.EnableOutputCapture()
	registerBaseLibraries(env, nil)
	registerServerFSPaths(env)

	cleanup := func() {}

	aiClient := createServerAIClient(client, opts.User)
	if aiClient != nil {
		env.SetObjectVar("ai_client", scriptlingai.WrapClient(aiClient))
	}

	var mcpLib *knotscriptling.MCPLibrary
	if client != nil && opts.User != nil {
		if opts.EventEnvelope == nil {
			mcpLib = knotscriptling.GetMCPLibraryInstance(client, opts.MCPParams)
		}
		registerKnotLibraries(env, client, opts.User.Id, opts.MCPParams, mcpLib, aiClient, false)
		env.RegisterLibrary(knotscriptling.GetIdentityLibrary(NewUserObject(opts.User)))

		// Server-side code — user-created tools and event sinks — reaches
		// plugin handlers only over the authenticated loopback: same
		// transport as the knot.* libraries, real web dispatch, declared
		// gates enforced. Never in-process.
		registerPluginLoopbackCallLibrary(env, client, opts.User)

		// The one deliberate exception to "no plugin code in this
		// environment": a plugin's declared export modules
		// ([tool.knot] export) are materialized here — each as
		// plugin.<name>.<stem>, the first also as plugin.<name> — client
		// SDKs whose classes wrap knot.plugin.call. They execute with the
		// caller's authority and every call rides the gated loopback
		// above, exactly like a hand-written call, so the declaration adds
		// ergonomics, never authority. Read and linted once at plugin
		// load; anything a plugin did not declare stays unattached.
		if registry := plugins.GetRegistry(); registry != nil {
			for _, p := range registry.All() {
				for i, ex := range p.Exports {
					stem := strings.TrimSuffix(filepath.Base(ex.Path), ".py")
					env.RegisterScriptLibrary("plugin."+p.ScriptNamespace+"."+stem, ex.Source)
					if i == 0 {
						env.RegisterScriptLibrary("plugin."+p.ScriptNamespace, ex.Source)
					}
				}
			}
		}

		if opts.EventEnvelope != nil {
			env.RegisterLibrary(knotscriptling.GetEventLibrary())
			if err := env.SetObjectVar(knotscriptling.EventParamsVarName, object.NewStringDict(opts.EventParams)); err != nil {
				cleanup()
				return nil, nil, nil, fmt.Errorf("failed to set event params: %v", err)
			}
			if err := env.SetObjectVar(knotscriptling.EventMetaVarName, buildEventMetaDict(opts.EventEnvelope)); err != nil {
				cleanup()
				return nil, nil, nil, fmt.Errorf("failed to set event metadata: %v", err)
			}
		}
		// User-created tools and event sinks are the untrusted domain: the
		// plugin pool is deliberately NOT attached to this environment, so a
		// user tool cannot import plugin code (plugin.<name>) at all. It
		// reaches a plugin only through the gated loopback above
		// (knot.plugin.call), where knot enforces the declared handler gate
		// before any plugin code runs. This is the structural isolation the
		// trust model rests on — the wall is "not attached", not a policy a
		// plugin could forget. Trusted plugin-to-plugin composition is a
		// separate surface (the plugin handler pool), unaffected.

		env.SetLibraryLoader(libloader.NewChain(
			newKnotLibsLoader(),
			newServerLibraryLoaderWithContext(client, opts.User),
		))
	}

	return env, mcpLib, cleanup, nil
}

// registerPluginLibraries registers the library surface for plugin handler
// environments: the full local-processing set — stdlib, base extended
// libraries, and the system-access libraries (os, subprocess, fs, …) that
// server envs deny — with every path-taking library jailed to the plugin's
// own folder. Deliberately absent: the outbound networking libraries
// (requests, scriptling.wait_for), the runtime libraries
// (scriptling.container, scriptling.nomad), machine-provisioning
// (scriptling.provision.*, agent only), and scriptling.ai.memory (AI
// scratchpad, agent only) — plugins are trusted for local compute, not for
// reaching out or driving container runtimes. A plugin that needs a database
// ships the driver as a bin/ peer, not an env library. Keep this list in step
// with plugins.pluginEnvLibraries, which answers metadata dependency
// resolution for the same set.
func registerPluginLibraries(env *scriptling.Scriptling, pluginDir string, log logger.Logger) {
	stdlib.RegisterAll(env)

	aux := log
	if aux == nil {
		aux = logger.NewNullLogger()
	}

	allowed := []string{pluginDir}
	extlibs.RegisterSecretsLibrary(env)
	extlibs.RegisterYAMLLibrary(env)
	extlibs.RegisterTOMLLibrary(env)
	extlibs.RegisterHTMLParserLibrary(env)
	extlibs.RegisterLoggingLibraryDefault(env)
	extlibs.RegisterShlexLibrary(env)
	extlibs.RegisterCsvLibrary(env)
	extlibs.RegisterXmlLibrary(env)
	extlibs.RegisterTemplateHTMLLibrary(env)
	extlibs.RegisterTemplateTextLibrary(env)

	scriptlingai.Register(env)
	agent.Register(env)
	scriptlingaitools.Register(env)
	scriptlingsimilarity.Register(env)
	scriptlingmcp.Register(env)
	scriptlingmcp.RegisterToon(env)
	scriptlingmcp.RegisterToolHelpers(env)

	telegram.Register(env, aux)
	discord.Register(env, aux)
	slack.Register(env, aux)

	extlibs.RegisterOSLibrary(env, allowed)
	extlibs.RegisterSubprocessLibrary(env)
	extlibs.RegisterPathlibLibrary(env, allowed)
	extlibs.RegisterGlobLibrary(env, allowed)
	extlibs.RegisterTempfileLibrary(env, allowed)
	extlibs.RegisterShutilLibrary(env, allowed)
	extlibs.RegisterZipfileLibrary(env, allowed)
	extlibs.RegisterTarfileLibrary(env, allowed)
	extlibs.RegisterFSLibrary(env, allowed)
	extlibs.RegisterGrepLibrary(env, allowed)
	extlibs.RegisterFindLibrary(env, allowed)
	extlibs.RegisterSedLibrary(env, allowed)
}

// NewPluginScriptlingEnv creates the environment a plugin's handlers run in
// when dispatched for a user: registerPluginLibraries jailed to the plugin's
// folder, knot's run-as-user libraries over the loopback API, the plugin's
// binary peers as plugin.* libraries, and a loader chain that resolves the
// plugin's own modules first, then knot's embedded libraries, then the
// invoking user's lib scripts.
func NewPluginScriptlingEnv(client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin) (*scriptling.Scriptling, error) {
	if client == nil || user == nil {
		return nil, fmt.Errorf("plugin env requires a client and user")
	}
	env := buildPluginEnv(plugin)
	if err := bindPluginUserIdentity(env, client, user, plugin); err != nil {
		return nil, err
	}
	return env, nil
}

// buildPluginEnv constructs the user-independent half of a plugin
// environment: the jailed library surface, this plugin's own handler
// namespace, and — because installed plugins are one trust domain — every
// other installed plugin's exports for cross-plugin composition. It is the
// reusable part; the pooled envs keep it across leases.
//
// The trust model (docs/plugin-architecture.md): installed = trusted.
// Plugins share one execution domain and compose freely — a handler imports
// another plugin's library or class as plugin.<name> and uses it ungated.
// This is the shared trusted pool. The isolation boundary is structural:
// this whole surface is simply NOT attached to the MCP-tool environment
// (NewServerScriptlingEnv), so untrusted user tools cannot import plugin
// code and reach a plugin only through the gated loopback.
func buildPluginEnv(plugin *plugins.Plugin) *scriptling.Scriptling {
	env := scriptling.New()
	env.EnableOutputCapture()
	registerPluginLibraries(env, plugin.Dir, nil)

	// This plugin's own handler namespace: a pure-script plugin's main.py is
	// registered as the plugin.<ScriptNamespace> library so its declared
	// handlers are addressable as plugin.<ns>.<fn>, the same shape a peer's
	// exports take. A peer plugin has no EntrySource — its peer registers
	// plugin.<peer> below via the shared-pool pass.
	if plugin.EntrySource != "" {
		env.RegisterScriptLibrary("plugin."+plugin.ScriptNamespace, plugin.EntrySource)
	}

	// Every installed plugin's exports, this one included, under plugin.*:
	// binary peers over the plugin protocol, scriptling libs (libs/*.py) and
	// peer main.py handler namespaces as in-process script libraries. First
	// registrant by plugin name wins a collision, matching the read-only
	// aggregation the MCP env would do — but here it is the live trusted
	// surface handlers compose against, not a read-only import view.
	registerTrustedPluginPool(env, plugin)
	return env
}

// registerTrustedPluginPool registers every installed plugin's exported
// surface into a plugin handler environment: the shared trusted pool. self
// is the plugin whose env this is — its own main.py namespace is already
// registered by the caller, so it is skipped here to avoid a double
// registration, but its peers and libs are included through the normal
// pass. Name collisions resolve first-by-name (same policy as logos and the
// MCP read-only import aggregation).
func registerTrustedPluginPool(env *scriptling.Scriptling, self *plugins.Plugin) {
	registry := plugins.GetRegistry()
	if registry == nil {
		// No registry (unit tests build a lone plugin): fall back to just
		// this plugin's own peers and libs.
		if scope := self.Scope(); scope != nil {
			pluginpkg.RegisterLibraries(env, scope)
		}
		for _, lib := range self.Libs {
			env.RegisterScriptLibrary("plugin."+lib.Name, lib.Source)
		}
		return
	}

	seen := map[string]bool{}
	if self.ScriptNamespace != "" {
		seen["plugin."+self.ScriptNamespace] = true
	}
	for _, p := range registry.All() {
		// A peer plugin's handler namespace is plugin.<peer>, supplied by
		// its peer's client registration below; a pure-script plugin's is
		// plugin.<ScriptNamespace>. Register main.py namespaces for plugins
		// other than self (self's is registered by buildPluginEnv already).
		if p.Name != self.Name && p.EntrySource != "" {
			name := "plugin." + p.ScriptNamespace
			if !seen[name] {
				seen[name] = true
				env.RegisterScriptLibrary(name, p.EntrySource)
			}
		}
		for _, lib := range p.Libs {
			name := "plugin." + lib.Name
			if seen[name] {
				continue
			}
			seen[name] = true
			env.RegisterScriptLibrary(name, lib.Source)
		}
		if scope := p.Scope(); scope != nil {
			for _, meta := range scope.List() {
				// A peer's handshake name is already namespaced under
				// "plugin." (scriptling NamespacePrefix); normalize the
				// dedup key so it lines up with the plugin.<ns> keys above.
				name := "plugin." + strings.TrimPrefix(meta.Name, "plugin.")
				if seen[name] {
					continue
				}
				client, ok := scope.Get(meta.Name)
				if !ok {
					continue
				}
				seen[name] = true
				pluginpkg.RegisterClientLibrary(env, client)
			}
		}
	}
}

// bindPluginUserIdentity attaches the requesting user to a plugin env: the
// run-as-user knot.* transports (knot.ai, the apiclient transport, the MCP
// tools library — re-registered by name, replacing any previous user's
// binding), the ai_client var, and the loader chain ending in the user's
// lib scripts. Every surface that carries identity is bound here, so a
// pooled env rebound per lease runs entirely as its new user.
func bindPluginUserIdentity(env *scriptling.Scriptling, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin) error {
	aiClient := createServerAIClient(client, user)
	if aiClient != nil {
		if err := env.SetObjectVar("ai_client", scriptlingai.WrapClient(aiClient)); err != nil {
			return err
		}
	}
	registerKnotLibraries(env, client, user.Id, nil, nil, aiClient, false)
	registerPluginCallLibrary(env, client, user)
	// knot.identity is the authoritative identity surface for a plugin env.
	// A handler receives the caller as request["user"] data (passed per
	// call by the dispatcher), but module code — peers, lib scripts — never
	// sees a handler's request, so it reads the user through knot.identity.
	// Re-registered on every rebind so a pooled env answers as its current
	// user.
	env.RegisterLibrary(knotscriptling.GetIdentityLibrary(NewUserObject(user)))

	loaders := []libloader.LibraryLoader{newKnotLibsLoader(), libloader.NewFilesystem(plugin.Dir),
		newServerLibraryLoaderWithContext(client, user)}
	env.SetLibraryLoader(libloader.NewChain(loaders...))
	return nil
}

// AgentScriptlingOptions configures an agent-side (in-space) scriptling
// environment. Output semantics: nil Output captures the script's output for
// retrieval; io.Discard throws it away (system scripts); any other writer
// streams to it. Input, when set, is wired to input() and sys.stdin.
type AgentScriptlingOptions struct {
	Argv   []string
	Logger logger.Logger
	Output io.Writer
	Input  io.Reader
}

// NewAgentScriptlingEnv creates the scriptling environment for every script
// that runs in a space under the agent: startup/shutdown scripts, streaming
// user scripts (`knot space run-script`), `knot run-script` eval, `knot
// methods register`, and health check scripts. This is the full side of
// knot's two environments — registerAgentLibraries (the scriptling CLI
// surface minus container/nomad), knot libraries with knot.methods, and
// knot.healthcheck. Scripts already run inside the space container, so fs is
// unrestricted and the plugin scope allows both HTTP(S) and stdio executable
// plugins.
//
// Interactive decorations are per-call and layered by the caller: the console
// stub and scriptling.ai.agent.interact in execute_script_stream.go, and
// per-call argv/input on pooled environments.
func NewAgentScriptlingEnv(client *apiclient.ApiClient, userId string, opts AgentScriptlingOptions) (*scriptling.Scriptling, func(), error) {
	env := scriptling.New()
	if opts.Output != nil {
		env.SetOutputWriter(opts.Output)
	} else {
		env.EnableOutputCapture()
	}
	if opts.Input != nil {
		env.SetInputReader(opts.Input)
	}

	registerAgentLibraries(env, opts.Logger)

	aiClient := createServerAIClient(client, nil)
	registerKnotLibraries(env, client, userId, nil, nil, aiClient, true)
	env.RegisterLibrary(knotscriptling.GetHealthCheckLibrary())

	cleanup := func() {}

	setupLibraryLoader(env, client)
	extlibs.RegisterSysLibrary(env, opts.Argv, opts.Input)
	if opts.Input != nil {
		env.SetObjectVar("input", extlibs.NewInputBuiltin(opts.Input))
	}
	return env, cleanup, nil
}
