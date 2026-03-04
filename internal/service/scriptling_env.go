package service

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
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
	scriptlingfuzzy "github.com/paularlott/scriptling/extlibs/fuzzy"
	scriptlingmcp "github.com/paularlott/scriptling/extlibs/mcp"
	"github.com/paularlott/scriptling/libloader"
	"github.com/paularlott/scriptling/object"
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
	if customLogger != nil {
		extlibs.RegisterLoggingLibrary(env, customLogger)
	} else {
		extlibs.RegisterLoggingLibraryDefault(env)
	}

	scriptlingai.Register(env)
	agent.Register(env)
	scriptlingaitools.Register(env)
	scriptlingfuzzy.Register(env)
	scriptlingmcp.Register(env)
	scriptlingmcp.RegisterToon(env)
	scriptlingmcp.RegisterToolHelpers(env)
	extlibs.RegisterTOMLLibrary(env)
}

// registerKnotLibraries registers all Knot-specific libraries for scriptling environments
// If mcpLib is provided, it will be registered instead of creating a new one via GetMCPToolsLibrary
// aiClient may be nil for local/remote environments where no AI client is available
func registerKnotLibraries(env *scriptling.Scriptling, client *apiclient.ApiClient, userId string, mcpParams map[string]object.Object, mcpLib *knotscriptling.MCPLibrary, aiClient ai.Client) {
	// knot.ai is always registered - Client() will return error if aiClient is nil
	env.RegisterLibrary(knotscriptling.GetAILibrary(aiClient))

	if client != nil && userId != "" {
		env.RegisterLibrary(knotscriptling.GetSpacesLibrary(client, userId))
		env.RegisterLibrary(knotscriptling.GetUsersLibrary(client, userId))
		env.RegisterLibrary(knotscriptling.GetGroupsLibrary(client, userId))
		env.RegisterLibrary(knotscriptling.GetRolesLibrary(client, userId))
		env.RegisterLibrary(knotscriptling.GetTemplatesLibrary(client, userId))
		env.RegisterLibrary(knotscriptling.GetVarsLibrary(client, userId))
		env.RegisterLibrary(knotscriptling.GetVolumesLibrary(client, userId))
		env.RegisterLibrary(knotscriptling.GetSkillsLibrary(client, userId))
		env.RegisterLibrary(knotscriptling.GetPermissionLibrary(client))
	}
	if client != nil {
		if mcpLib != nil {
			env.RegisterLibrary(mcpLib.GetLibrary())
		} else {
			env.RegisterLibrary(knotscriptling.GetMCPToolsLibrary(client, mcpParams))
		}
	}
}

// registerFullSystemLibraries registers system access libraries (subprocess, os, pathlib, scriptling.threads, scriptling.console, scriptling.glob)
// and interactive agent support
func registerFullSystemLibraries(env *scriptling.Scriptling) {
	extlibs.RegisterSubprocessLibrary(env)

	// Register only the core runtime library (background function)
	extlibs.RegisterRuntimeLibrary(env)
	extlibs.RegisterRuntimeKVLibrary(env)   // Key-value store
	extlibs.RegisterRuntimeSyncLibrary(env) // Concurrency primitives

	scriptlingconsole.Register(env) // scriptling.console
	extlibs.RegisterOSLibrary(env, nil)
	extlibs.RegisterPathlibLibrary(env, nil)
	extlibs.RegisterGlobLibrary(env, nil) // scriptling.glob
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

// setupLibraryLoader sets up library loading from configured libdir and/or server
func setupLibraryLoader(env *scriptling.Scriptling, client *apiclient.ApiClient) {
	var loaders []libloader.LibraryLoader

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

	if len(loaders) > 0 {
		env.SetLibraryLoader(libloader.NewChain(loaders...))
	}
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

// buildLocalLibDirs constructs the ordered list of library search directories for local execution,
// mirroring scriptling-cli behaviour: script dir (or cwd) first, then extra paths, then configured libdir.
func buildLocalLibDirs(scriptFile string, extraLibPaths []string) []string {
	var dirs []string

	// Script dir or cwd first
	if scriptFile != "" {
		dirs = append(dirs, filepath.Dir(scriptFile))
	} else {
		if cwd, err := os.Getwd(); err == nil {
			dirs = append(dirs, cwd)
		}
	}

	// Additional paths from --libpath
	for _, d := range extraLibPaths {
		if d != "" {
			dirs = append(dirs, d)
		}
	}

	// Configured libdir last
	cfg := config.GetServerConfig()
	if cfg != nil && cfg.LibDir != "" {
		dirs = append(dirs, cfg.LibDir)
	}

	return dirs
}

// NewLocalScriptlingEnv creates a scriptling environment for local execution on desktop/agent.
// scriptFile is the path to the script being run (used to derive the lib search dir); pass "" for stdin/interactive.
// extraLibPaths are additional directories to search for libraries (e.g. from --libpath flags).
// Libraries: stdlib, requests, secrets, subprocess, htmlparser, threads, os, pathlib, sys, knot.space, knot.ai, knot.mcp
// On-demand loading: script dir → extra paths → libdir → server API
// Output: Uses stdin/stdout directly with zero buffering
func NewLocalScriptlingEnv(argv []string, client *apiclient.ApiClient, userId string, scriptFile string, extraLibPaths []string) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	env.SetOutputWriter(os.Stdout)
	registerBaseLibraries(env, nil)
	registerFullSystemLibraries(env)
	agent.RegisterInteract(env)

	// Create AI client that connects to the server's OpenAI endpoint
	aiClient := createServerAIClient(client, nil)

	registerKnotLibraries(env, client, userId, nil, nil, aiClient)

	// Set up library loader chain: script dir → extra paths → libdir → server API → fetcher
	var loaders []libloader.LibraryLoader

	for _, dir := range buildLocalLibDirs(scriptFile, extraLibPaths) {
		loaders = append(loaders, libloader.NewFilesystem(dir))
	}

	// Add server API loader or fetcher loader
	if client != nil {
		loaders = append(loaders, newServerLibraryLoader(client))
	} else if libraryFetcher != nil {
		loaders = append(loaders, newFetcherLoader())
	}

	if len(loaders) > 0 {
		env.SetLibraryLoader(libloader.NewChain(loaders...))
	}

	extlibs.RegisterSysLibrary(env, argv, os.Stdin)
	return env, nil
}

// NewMCPScriptlingEnv creates a scriptling environment for MCP tool execution
// Libraries: stdlib, requests, secrets, htmlparser, knot.space, knot.ai, knot.mcp, knot.user, knot.group, knot.role, knot.template, knot.vars, knot.volume, knot.permission
// On-demand loading: Enabled - fetches from server only
// Output: Captured and returned
// The AI client connects to the server's OpenAI-compatible endpoint via createServerAIClient.
// The MCPServerContext middleware handles per-user tool discovery and execution when
// requests flow through the endpoint.
// Returns the environment and the MCP library instance for result retrieval
func NewMCPScriptlingEnv(client *apiclient.ApiClient, mcpParams map[string]object.Object, user *model.User) (*scriptling.Scriptling, *knotscriptling.MCPLibrary, error) {
	env := scriptling.New()
	env.EnableOutputCapture()
	registerBaseLibraries(env, nil)

	// Create AI client that connects to the server's OpenAI endpoint
	aiClient := createServerAIClient(client, user)

	// Inject AI client as global variable for scriptling.ai
	if aiClient != nil {
		env.SetObjectVar("ai_client", scriptlingai.WrapClient(aiClient))
	}

	var mcpLib *knotscriptling.MCPLibrary
	if client != nil && user != nil {
		mcpLib = knotscriptling.GetMCPLibraryInstance(client, mcpParams)
		registerKnotLibraries(env, client, user.Id, mcpParams, mcpLib, aiClient)

		// Set up library loader with user context for MuxClient
		env.SetLibraryLoader(newServerLibraryLoaderWithContext(client, user))
	}

	return env, mcpLib, nil
}

// NewRemoteScriptlingEnv creates a scriptling environment for remote execution in spaces
// Libraries: stdlib, requests, secrets, subprocess, htmlparser, threads, os, pathlib, sys, knot.space, knot.ai, knot.mcp
// On-demand loading: Enabled - fetches from server only
// customLogger is optional - pass nil to use the default logger
// Output: Captured and returned for user scripts, discarded for system scripts (startup/shutdown)
func NewRemoteScriptlingEnv(argv []string, client *apiclient.ApiClient, userId string, customLogger logger.Logger, isSystemCall bool) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	if isSystemCall {
		env.SetOutputWriter(io.Discard)
	} else {
		env.EnableOutputCapture()
	}
	registerBaseLibraries(env, customLogger)
	registerFullSystemLibraries(env)

	aiClient := createServerAIClient(client, nil)
	registerKnotLibraries(env, client, userId, nil, nil, aiClient)

	setupLibraryLoader(env, client)
	extlibs.RegisterSysLibrary(env, argv, nil)
	return env, nil
}

// NewRemoteStreamingScriptlingEnv creates a scriptling environment for streaming remote execution
// Libraries: stdlib, requests, secrets, subprocess, htmlparser, threads, os, pathlib, sys, knot.space, knot.ai, knot.mcp
// Note: scriptling.console and scriptling.ai.agent.interact are registered after env creation in execute_script_stream.go
// On-demand loading: Enabled - fetches from server only
// customLogger is optional - pass nil to use the default logger
// Output: Connected to provided writer, input from provided reader
func NewRemoteStreamingScriptlingEnv(argv []string, client *apiclient.ApiClient, userId string, customLogger logger.Logger, output io.Writer, input io.Reader) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	env.SetOutputWriter(output)
	env.SetInputReader(input)
	registerBaseLibraries(env, customLogger)

	extlibs.RegisterSubprocessLibrary(env)
	extlibs.RegisterRuntimeLibrary(env)
	extlibs.RegisterRuntimeKVLibrary(env)
	extlibs.RegisterRuntimeSyncLibrary(env)
	// scriptling.console intentionally not registered here — registered via registerConsoleStub in execute_script_stream.go
	// scriptling.ai.agent.interact intentionally not registered here — registered via agent.RegisterInteract in execute_script_stream.go
	extlibs.RegisterOSLibrary(env, nil)
	extlibs.RegisterPathlibLibrary(env, nil)
	extlibs.RegisterGlobLibrary(env, nil)

	aiClient := createServerAIClient(client, nil)
	registerKnotLibraries(env, client, userId, nil, nil, aiClient)

	setupLibraryLoader(env, client)
	extlibs.RegisterSysLibrary(env, argv, input)
	if input != nil {
		env.SetObjectVar("input", extlibs.NewInputBuiltin(input))
	}
	return env, nil
}

// RunScript executes a script with local environment.
// scriptFile is the path to the script file on disk (used for lib path resolution); pass "" if not applicable.
// extraLibPaths are additional directories to search for libraries.
func RunScript(ctx context.Context, scriptContent string, argv []string, client *apiclient.ApiClient, userId string, scriptFile string, extraLibPaths []string) (string, error) {
	env, err := NewLocalScriptlingEnv(argv, client, userId, scriptFile, extraLibPaths)
	if err != nil {
		return "", err
	}

	result, err := env.Eval(scriptContent)
	ExitOnSystemExit(result)

	if err != nil {
		return "", err
	}

	if result != nil && result.Inspect() != "None" {
		return result.Inspect(), nil
	}

	return "", nil
}
