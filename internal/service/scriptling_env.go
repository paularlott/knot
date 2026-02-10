package service

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
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
	scriptlingmcp "github.com/paularlott/scriptling/extlibs/mcp"
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
	scriptlingmcp.Register(env)
	scriptlingmcp.RegisterToon(env)
	scriptlingmcp.RegisterToolHelpers(env)
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
	extlibs.RegisterThreadsLibrary(env) // scriptling.threads
	extlibs.RegisterConsoleLibrary(env) // scriptling.console
	extlibs.RegisterOSLibrary(env, []string{})
	extlibs.RegisterPathlibLibrary(env, []string{})
	extlibs.RegisterGlobLibrary(env, []string{}) // scriptling.glob
	agent.RegisterInteract(env)                  // scriptling.ai.agent.interact (extends Agent with interact())
}

// setupServerLibraryCallback sets up on-demand library loading from server
func setupServerLibraryCallback(env *scriptling.Scriptling, client *apiclient.ApiClient) {
	if client != nil {
		env.SetOnDemandLibraryCallback(func(p *scriptling.Scriptling, libName string) bool {
			// Use background context with timeout for library loading
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			content, err := client.GetScriptLibrary(ctx, libName)
			if err == nil {
				return p.RegisterScriptLibrary(libName, content) == nil
			}
			return false
		})
	} else if libraryFetcher != nil {
		env.SetOnDemandLibraryCallback(func(p *scriptling.Scriptling, libName string) bool {
			content, err := libraryFetcher(libName)
			if err == nil {
				return p.RegisterScriptLibrary(libName, content) == nil
			}
			return false
		})
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

// NewLocalScriptlingEnv creates a scriptling environment for local execution on desktop/agent
// Libraries: stdlib, requests, secrets, subprocess, htmlparser, threads, os, pathlib, sys, knot.space, knot.ai, knot.mcp
// On-demand loading: Enabled - tries local .py files first, then fetches from server
// Output: Uses stdin/stdout directly with zero buffering
func NewLocalScriptlingEnv(argv []string, client *apiclient.ApiClient, userId string) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	env.SetOutputWriter(os.Stdout)
	registerBaseLibraries(env, nil)
	registerFullSystemLibraries(env)

	// Create AI client that connects to the server's OpenAI endpoint
	aiClient := createServerAIClient(client, nil)

	registerKnotLibraries(env, client, userId, nil, nil, aiClient)

	// Local-first library loading: try filesystem, then server
	env.SetOnDemandLibraryCallback(func(p *scriptling.Scriptling, libName string) bool {
		if content, err := os.ReadFile(libName + ".py"); err == nil {
			return p.RegisterScriptLibrary(libName, string(content)) == nil
		}
		if client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if content, err := client.GetScriptLibrary(ctx, libName); err == nil {
				return p.RegisterScriptLibrary(libName, content) == nil
			}
		} else if libraryFetcher != nil {
			if content, err := libraryFetcher(libName); err == nil {
				return p.RegisterScriptLibrary(libName, content) == nil
			}
		}
		return false
	})

	extlibs.RegisterSysLibrary(env, argv)
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

		// Set up library callback with user context for MuxClient
		env.SetOnDemandLibraryCallback(func(p *scriptling.Scriptling, libName string) bool {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ctx = context.WithValue(ctx, "user", user)

			content, err := client.GetScriptLibrary(ctx, libName)
			if err == nil {
				return p.RegisterScriptLibrary(libName, content) == nil
			}
			return false
		})
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

	setupServerLibraryCallback(env, client)
	extlibs.RegisterSysLibrary(env, argv)
	return env, nil
}

// NewRemoteStreamingScriptlingEnv creates a scriptling environment for streaming remote execution
// Libraries: stdlib, requests, secrets, subprocess, htmlparser, threads, os, pathlib, sys, knot.space, knot.ai, knot.mcp
// On-demand loading: Enabled - fetches from server only
// customLogger is optional - pass nil to use the default logger
// Output: Connected to provided writer, input from provided reader
func NewRemoteStreamingScriptlingEnv(argv []string, client *apiclient.ApiClient, userId string, customLogger logger.Logger, output io.Writer, input io.Reader) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	env.SetOutputWriter(output)
	env.SetInputReader(input)
	registerBaseLibraries(env, customLogger)
	registerFullSystemLibraries(env)

	aiClient := createServerAIClient(client, nil)
	registerKnotLibraries(env, client, userId, nil, nil, aiClient)

	setupServerLibraryCallback(env, client)
	extlibs.RegisterSysLibrary(env, argv)
	return env, nil
}

// RunScript executes a script with local environment
func RunScript(ctx context.Context, scriptContent string, argv []string, client *apiclient.ApiClient, userId string) (string, error) {
	env, err := NewLocalScriptlingEnv(argv, client, userId)
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
