package service

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/openai"
	knotscriptling "github.com/paularlott/knot/internal/scriptling"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/extlibs"
	"github.com/paularlott/scriptling/stdlib"
)

var (
	openaiClient     *openai.Client
	openaiClientOnce sync.Once
	libraryFetcher   func(string) (string, error)
)

// SetOpenAIClient sets the global OpenAI client for scriptling environments
func SetOpenAIClient(client *openai.Client) {
	openaiClientOnce.Do(func() {
		openaiClient = client
	})
}

// GetOpenAIClient returns the global OpenAI client
func GetOpenAIClient() *openai.Client {
	return openaiClient
}

// SetLibraryFetcher sets a callback for fetching libraries when API client is not available
func SetLibraryFetcher(fetcher func(string) (string, error)) {
	libraryFetcher = fetcher
}

// registerBaseLibraries registers common libraries shared across all environments
func registerBaseLibraries(env *scriptling.Scriptling) {
	stdlib.RegisterAll(env)
	extlibs.RegisterRequestsLibrary(env)
	extlibs.RegisterSecretsLibrary(env)
	extlibs.RegisterHTMLParserLibrary(env)
	extlibs.RegisterWaitForLibrary(env)
	env.EnableOutputCapture()
}

// registerFullSystemLibraries registers system access libraries (subprocess, os, pathlib)
func registerFullSystemLibraries(env *scriptling.Scriptling) {
	extlibs.RegisterSubprocessLibrary(env)
	extlibs.RegisterThreadsLibrary(env)
	extlibs.RegisterOSLibrary(env, []string{})
	extlibs.RegisterPathlibLibrary(env, []string{})
}

// setupServerLibraryCallback sets up on-demand library loading from server
func setupServerLibraryCallback(env *scriptling.Scriptling, client *apiclient.ApiClient) {
	if client != nil {
		env.SetOnDemandLibraryCallback(func(p *scriptling.Scriptling, libName string) bool {
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

// NewLocalScriptlingEnv creates a scriptling environment for local execution on desktop/agent
// Libraries: stdlib, requests, secrets, subprocess, htmlparser, threads, os, pathlib, sys, spaces, ai, mcp
// On-demand loading: Enabled - tries local .py files first, then fetches from server
func NewLocalScriptlingEnv(argv []string, client *apiclient.ApiClient, userId string) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	registerBaseLibraries(env)
	registerFullSystemLibraries(env)

	if client != nil && userId != "" {
		env.RegisterLibrary("spaces", knotscriptling.GetSpacesLibrary(client, userId))
		env.RegisterLibrary("ai", knotscriptling.GetAILibrary(client, userId))
	}
	if client != nil {
		env.RegisterLibrary("mcp", knotscriptling.GetMCPToolsLibrary(client))
	}

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
// Libraries: stdlib, requests, secrets, htmlparser, spaces, ai
// On-demand loading: Enabled - fetches from server only
func NewMCPScriptlingEnv(client *apiclient.ApiClient, mcpParams map[string]string, user *model.User) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	registerBaseLibraries(env)

	if user != nil {
		env.RegisterLibrary("spaces", knotscriptling.GetSpacesMCPLibrary(user, GetSpaceService(), GetContainerService(), nil, ExecuteScriptLocally))
	}
	if GetOpenAIClient() != nil && user != nil {
		env.RegisterLibrary("ai", knotscriptling.GetAIMCPLibrary(GetOpenAIClient()))
	}

	setupServerLibraryCallback(env, client)
	return env, nil
}

// NewRemoteScriptlingEnv creates a scriptling environment for remote execution in spaces
// Libraries: stdlib, requests, secrets, subprocess, htmlparser, threads, os, pathlib, sys, spaces, ai, mcp
// On-demand loading: Enabled - fetches from server only
func NewRemoteScriptlingEnv(argv []string, client *apiclient.ApiClient, userId string) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	registerBaseLibraries(env)
	registerFullSystemLibraries(env)

	if client != nil && userId != "" {
		env.RegisterLibrary("spaces", knotscriptling.GetSpacesLibrary(client, userId))
		env.RegisterLibrary("ai", knotscriptling.GetAILibrary(client, userId))
	}
	if client != nil {
		env.RegisterLibrary("mcp", knotscriptling.GetMCPToolsLibrary(client))
	}

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
	if err != nil {
		return "", err
	}

	output := env.GetOutput()
	if result != nil && result.Inspect() != "None" {
		if output != "" {
			output += "\n"
		}
		output += result.Inspect()
	}

	return output, nil
}
