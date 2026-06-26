package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/dns"
	knotscriptling "github.com/paularlott/knot/internal/scriptling"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/logger"
	ai "github.com/paularlott/mcp/ai"
	mcpopenai "github.com/paularlott/mcp/ai/openai"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/extlibs"
	"github.com/paularlott/scriptling/extlibs/agent"
	scriptlingai "github.com/paularlott/scriptling/extlibs/ai"
	scriptlingaitools "github.com/paularlott/scriptling/extlibs/ai/tools"
	scriptlingconsole "github.com/paularlott/scriptling/extlibs/console"
	scriptlingmcp "github.com/paularlott/scriptling/extlibs/mcp"
	scriptlingresolve "github.com/paularlott/scriptling/extlibs/net/resolve"
	provisionfetch "github.com/paularlott/scriptling/extlibs/provision/fetch"
	provisionfile "github.com/paularlott/scriptling/extlibs/provision/file"
	scriptlingsimilarity "github.com/paularlott/scriptling/extlibs/similarity"
	"github.com/paularlott/scriptling/libloader"
	"github.com/paularlott/scriptling/object"
	"github.com/paularlott/scriptling/plugin"
	scriptlingsetup "github.com/paularlott/scriptling/scriptling-cli/setup"
	"github.com/paularlott/scriptling/stdlib"
)

var (
	libraryFetcher func(string) (string, error)
)

// registerBaseLibraries registers common libraries shared across all environments
// customLogger is optional - pass nil to use the default logger
func registerBaseLibraries(env *scriptling.Scriptling, customLogger logger.Logger) {
	stdlib.RegisterAll(env)
	extlibs.RegisterRequestsLibrary(env)
	extlibs.RegisterSecretsLibrary(env)
	extlibs.RegisterHTMLParserLibrary(env)
	extlibs.RegisterWaitForLibrary(env)
	extlibs.RegisterYAMLLibrary(env)
	extlibs.RegisterFSLibrary(env, nil)
	extlibs.RegisterMarkdownLibrary(env)
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
	scriptlingresolve.Register(env, dns.GetDefaultResolver())
	extlibs.RegisterTOMLLibrary(env)
	extlibs.RegisterWebSocketLibrary(env)
	extlibs.RegisterTemplateHTMLLibrary(env)
	extlibs.RegisterTemplateTextLibrary(env)
}

// registerKnotLibraries registers all Knot-specific libraries for scriptling environments.
// If mcpLib is provided, it will be registered instead of creating a new one via GetMCPToolsLibrary.
// aiClient may be nil for local/remote environments where no AI client is available.
// withMethods controls whether knot.methods / knot.methods.schema are registered —
// false for MCP tool execution envs (method registration is an agent-side concern).
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

// registerFullSystemLibraries registers system access libraries (subprocess, os, pathlib, scriptling.threads, scriptling.console, scriptling.glob, scriptling.grep, scriptling.sed)
// and interactive agent support
func registerFullSystemLibraries(env *scriptling.Scriptling) {
	extlibs.RegisterSubprocessLibrary(env)

	// Register only the core runtime library (background function)
	extlibs.RegisterRuntimeLibrary(env)
	extlibs.RegisterRuntimeKVLibrary(env)           // Key-value store
	extlibs.RegisterRuntimeSyncLibrary(env)         // Concurrency primitives
	extlibs.RegisterRuntimeSandboxLibrary(env, nil) // Sandbox execution (nil = no path restrictions)

	scriptlingconsole.Register(env)       // scriptling.console
	extlibs.RegisterGrepLibrary(env, nil) // scriptling.grep
	extlibs.RegisterSedLibrary(env, nil)  // scriptling.sed
	extlibs.RegisterOSLibrary(env, nil)
	extlibs.RegisterPathlibLibrary(env, nil)
	extlibs.RegisterGlobLibrary(env, nil) // scriptling.glob
	provisionfile.Register(env)
	provisionfetch.Register(env)
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

// NewMCPScriptlingEnv creates a scriptling environment for MCP tool execution
// Libraries: stdlib, requests, secrets, htmlparser, knot.space, knot.ai, knot.mcp, knot.user, knot.group, knot.role, knot.template, knot.vars, knot.volume, knot.permission
// On-demand loading: Enabled - fetches from server only
// Output: Captured and returned
// The AI client connects to the server's OpenAI-compatible endpoint via createServerAIClient.
// The MCPServerContext middleware handles per-user tool discovery and execution when
// requests flow through the endpoint.
// Returns the environment, the MCP library instance for result retrieval, and a cleanup
// function that must be called (e.g. via defer) once the script has finished executing
// to release the per-execution plugin scope. The plugin scope is HTTP-only: scripts may
// load remote HTTP(S) plugin endpoints via scriptling.plugin.load() but cannot spawn
// local executables, and plugins loaded by one execution are isolated from every other.
func NewMCPScriptlingEnv(client *apiclient.ApiClient, mcpParams map[string]object.Object, user *model.User) (*scriptling.Scriptling, *knotscriptling.MCPLibrary, func(), error) {
	env := scriptling.New()
	env.EnableOutputCapture()
	registerBaseLibraries(env, nil)

	// Register a per-execution plugin scope (HTTP-only) so scripts can call
	// scriptling.plugin.load() for remote HTTP(S) plugins without leaking
	// plugins between users/executions or spawning local executables.
	pluginScope := registerPluginScope(env, plugin.TransportHTTP)
	cleanup := func() { _ = pluginScope.Close() }

	// Create AI client that connects to the server's OpenAI endpoint
	aiClient := createServerAIClient(client, user)

	// Inject AI client as global variable for scriptling.ai
	if aiClient != nil {
		env.SetObjectVar("ai_client", scriptlingai.WrapClient(aiClient))
	}

	var mcpLib *knotscriptling.MCPLibrary
	if client != nil && user != nil {
		mcpLib = knotscriptling.GetMCPLibraryInstance(client, mcpParams)
		registerKnotLibraries(env, client, user.Id, mcpParams, mcpLib, aiClient, false)

		// Set up library loader with knot libs + user context for MuxClient
		env.SetLibraryLoader(libloader.NewChain(
			newKnotLibsLoader(),
			newServerLibraryLoaderWithContext(client, user),
		))
	}

	return env, mcpLib, cleanup, nil
}

// NewHealthCheckScriptlingEnv creates a minimal scriptling environment for health check scripts.
// Registers the knot.healthcheck built-in library only — no system access, no API client.
// Returns the environment and a cleanup function that must be called (e.g. via defer)
// once the script has finished executing to release the per-execution plugin scope.
// The plugin scope is HTTP-only: health checks may probe remote HTTP(S) plugin
// endpoints but cannot spawn local executables.
func NewHealthCheckScriptlingEnv() (*scriptling.Scriptling, func(), error) {
	env := scriptling.New()
	env.EnableOutputCapture()
	stdlib.RegisterAll(env)
	env.RegisterLibrary(knotscriptling.GetHealthCheckLibrary())

	pluginScope := registerPluginScope(env, plugin.TransportHTTP)
	cleanup := func() { _ = pluginScope.Close() }
	return env, cleanup, nil
}

// NewRemoteScriptlingEnv creates a scriptling environment for remote execution in spaces
// Libraries: stdlib, requests, secrets, subprocess, htmlparser, threads, os, pathlib, sys, scriptling.grep, scriptling.sed, knot.space, knot.ai, knot.mcp
// On-demand loading: Enabled - fetches from server only
// customLogger is optional - pass nil to use the default logger
// Output: Captured and returned for user scripts, discarded for system scripts (startup/shutdown)
// Returns the environment and a cleanup function that must be called (e.g. via defer)
// once the script has finished executing to release the per-execution plugin scope. The
// plugin scope allows both HTTP(S) and stdio executable plugins (space-side scripts already
// have subprocess access) but plugins loaded by one execution are isolated from every other.
func NewRemoteScriptlingEnv(argv []string, client *apiclient.ApiClient, userId string, customLogger logger.Logger, isSystemCall bool) (*scriptling.Scriptling, func(), error) {
	env := scriptling.New()
	if isSystemCall {
		env.SetOutputWriter(io.Discard)
	} else {
		env.EnableOutputCapture()
	}
	registerBaseLibraries(env, customLogger)
	registerFullSystemLibraries(env)

	aiClient := createServerAIClient(client, nil)
	registerKnotLibraries(env, client, userId, nil, nil, aiClient, true)

	pluginScope := registerPluginScope(env, plugin.TransportAll)
	cleanup := func() { _ = pluginScope.Close() }

	setupLibraryLoader(env, client)
	extlibs.RegisterSysLibrary(env, argv, nil)
	return env, cleanup, nil
}

// NewRunScriptEvalEnv builds the environment for `knot run-script` when it
// evaluates a script (not serving). It registers the full scriptling CLI
// library set MINUS the container library (no docker/podman runtime inside a
// space), then layers knot's own libraries on top — so plain run-script has the
// same library surface as the scriptling CLI and as run-script's server modes.
func NewRunScriptEvalEnv(argv []string, client *apiclient.ApiClient, userId string, customLogger logger.Logger, output io.Writer, input io.Reader) (*scriptling.Scriptling, func(), error) {
	env := scriptling.New()
	env.SetOutputWriter(output)
	env.SetInputReader(input)

	log := customLogger
	if log == nil {
		log = logger.NewNullLogger()
	}

	// Full scriptling CLI library set, minus container (no docker/podman).
	scriptlingsetup.Scriptling(env, nil, false, nil, []string{extlibs.ContainerLibraryName}, nil, log, "", "")

	// setup registers net.resolve with the default resolver; re-register with
	// knot's configured DNS resolver.
	scriptlingresolve.Register(env, dns.GetDefaultResolver())

	// Knot-specific libraries (knot.apiclient, knot.event, knot.ai, knot.methods…).
	aiClient := createServerAIClient(client, nil)
	registerKnotLibraries(env, client, userId, nil, nil, aiClient, true)
	env.RegisterLibrary(knotscriptling.GetHealthCheckLibrary())

	pluginScope := registerPluginScope(env, plugin.TransportAll)
	cleanup := func() { _ = pluginScope.Close() }

	setupLibraryLoader(env, client)
	extlibs.RegisterSysLibrary(env, argv, input)
	if input != nil {
		env.SetObjectVar("input", extlibs.NewInputBuiltin(input))
	}
	return env, cleanup, nil
}

// RegisterKnotServeLibraries adds knot's libraries to an environment created by
// the scriptling server runtime (used as the ServerConfig.ExtraLibs hook for
// `knot run-script` server modes). It registers the Go-backed knot libraries
// (knot.apiclient transport, knot.ai, knot.mcptools, knot.healthcheck) and
// chains knot's Python-library loader in front of the server's existing loader
// so `import knot.space` etc. resolve without losing the server's handler-module
// loader. knot.methods is intentionally not registered — in server mode the
// script is the method server itself (via scriptling.runtime.jsonrpc).
func RegisterKnotServeLibraries(env *scriptling.Scriptling, client *apiclient.ApiClient, userId string) {
	aiClient := createServerAIClient(client, nil)
	registerKnotLibraries(env, client, userId, nil, nil, aiClient, false)
	env.RegisterLibrary(knotscriptling.GetHealthCheckLibrary())

	knotLoader := newKnotLibsLoader()
	if existing := env.GetLibraryLoader(); existing != nil {
		env.SetLibraryLoader(libloader.NewChain(knotLoader, existing))
	} else {
		env.SetLibraryLoader(knotLoader)
	}
}

// NewRemoteStreamingScriptlingEnv creates a scriptling environment for streaming remote execution
// Libraries: stdlib, requests, secrets, subprocess, htmlparser, threads, os, pathlib, sys, scriptling.grep, scriptling.sed, knot.space, knot.ai, knot.mcp
// Note: scriptling.console and scriptling.ai.agent.interact are registered after env creation in execute_script_stream.go
// On-demand loading: Enabled - fetches from server only
// customLogger is optional - pass nil to use the default logger
// Output: Connected to provided writer, input from provided reader
// Returns the environment and a cleanup function that must be called (e.g. via defer)
// once the script has finished executing to release the per-execution plugin scope.
func NewRemoteStreamingScriptlingEnv(argv []string, client *apiclient.ApiClient, userId string, customLogger logger.Logger, output io.Writer, input io.Reader) (*scriptling.Scriptling, func(), error) {
	env := scriptling.New()
	env.SetOutputWriter(output)
	env.SetInputReader(input)
	registerBaseLibraries(env, customLogger)

	extlibs.RegisterSubprocessLibrary(env)
	extlibs.RegisterRuntimeLibrary(env)
	extlibs.RegisterRuntimeKVLibrary(env)
	extlibs.RegisterRuntimeSyncLibrary(env)
	extlibs.RegisterRuntimeSandboxLibrary(env, nil) // Sandbox execution (nil = no path restrictions)
	// scriptling.console intentionally not registered here — registered via registerConsoleStub in execute_script_stream.go
	// scriptling.ai.agent.interact intentionally not registered here — registered via agent.RegisterInteract in execute_script_stream.go
	extlibs.RegisterGrepLibrary(env, nil) // scriptling.grep
	extlibs.RegisterSedLibrary(env, nil)  // scriptling.sed
	extlibs.RegisterOSLibrary(env, nil)
	extlibs.RegisterPathlibLibrary(env, nil)
	extlibs.RegisterGlobLibrary(env, nil)
	provisionfile.Register(env)
	provisionfetch.Register(env)

	aiClient := createServerAIClient(client, nil)
	registerKnotLibraries(env, client, userId, nil, nil, aiClient, true)

	pluginScope := registerPluginScope(env, plugin.TransportAll)
	cleanup := func() { _ = pluginScope.Close() }

	setupLibraryLoader(env, client)
	extlibs.RegisterSysLibrary(env, argv, input)
	if input != nil {
		env.SetObjectVar("input", extlibs.NewInputBuiltin(input))
	}
	return env, cleanup, nil
}
