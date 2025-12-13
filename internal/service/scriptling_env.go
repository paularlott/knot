package service

import (
	"context"
	"os"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	knotscriptling "github.com/paularlott/knot/internal/scriptling"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/extlibs"
	"github.com/paularlott/scriptling/stdlib"
)

// NewLocalScriptlingEnv creates a scriptling environment for local execution on desktop/agent
// Libraries: All database libraries, stdlib, requests, secrets, subprocess, htmlparser, threads, os, pathlib, sys, spaces
// On-demand loading: Enabled for disk-based .py files
func NewLocalScriptlingEnv(argv []string, libraries map[string]string, client *apiclient.ApiClient, userId string) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	stdlib.RegisterAll(env)
	extlibs.RegisterRequestsLibrary(env)
	extlibs.RegisterSecretsLibrary(env)
	extlibs.RegisterSubprocessLibrary(env)
	extlibs.RegisterHTMLParserLibrary(env)
	extlibs.RegisterThreadsLibrary(env)
	extlibs.RegisterOSLibrary(env, []string{})
	extlibs.RegisterPathlibLibrary(env, []string{})
	env.EnableOutputCapture()

	registerScriptLibraries(env, libraries)

	if client != nil && userId != "" {
		env.RegisterLibrary("spaces", knotscriptling.GetSpacesLibrary(client, userId))
	}

	env.SetOnDemandLibraryCallback(func(p *scriptling.Scriptling, libName string) bool {
		filename := libName + ".py"
		content, err := os.ReadFile(filename)
		if err == nil {
			return p.RegisterScriptLibrary(libName, string(content)) == nil
		}
		return false
	})

	extlibs.RegisterSysLibrary(env, argv)
	return env, nil
}

// NewMCPScriptlingEnv creates a scriptling environment for MCP tool execution
// Libraries: All database libraries, MCP library, stdlib, requests, secrets, htmlparser, spaces
// On-demand loading: Disabled
func NewMCPScriptlingEnv(libraries map[string]string, mcpParams map[string]string, user *model.User) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	stdlib.RegisterAll(env)
	extlibs.RegisterRequestsLibrary(env)
	extlibs.RegisterSecretsLibrary(env)
	extlibs.RegisterHTMLParserLibrary(env)
	env.EnableOutputCapture()

	registerScriptLibraries(env, libraries)

	if user != nil {
		env.RegisterLibrary("spaces", knotscriptling.GetSpacesMCPLibrary(user, GetSpaceService(), GetContainerService(), nil, ExecuteScriptInSpace))
	}

	return env, nil
}

// NewRemoteScriptlingEnv creates a scriptling environment for remote execution in spaces
// Libraries: All database libraries, stdlib, requests, secrets, subprocess, htmlparser, threads, os, pathlib, sys, spaces
// On-demand loading: Disabled
func NewRemoteScriptlingEnv(argv []string, libraries map[string]string, client *apiclient.ApiClient, userId string) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	stdlib.RegisterAll(env)
	extlibs.RegisterRequestsLibrary(env)
	extlibs.RegisterSecretsLibrary(env)
	extlibs.RegisterSubprocessLibrary(env)
	extlibs.RegisterHTMLParserLibrary(env)
	extlibs.RegisterThreadsLibrary(env)
	extlibs.RegisterOSLibrary(env, []string{})
	extlibs.RegisterPathlibLibrary(env, []string{})
	env.EnableOutputCapture()

	registerScriptLibraries(env, libraries)

	if client != nil && userId != "" {
		env.RegisterLibrary("spaces", knotscriptling.GetSpacesLibrary(client, userId))
	}

	extlibs.RegisterSysLibrary(env, argv)
	return env, nil
}

func registerScriptLibraries(env *scriptling.Scriptling, libraries map[string]string) error {
	for name, content := range libraries {
		if err := env.RegisterScriptLibrary(name, content); err != nil {
			return err
		}
	}
	return nil
}

// RunScript executes a script with local environment
func RunScript(ctx context.Context, scriptContent string, argv []string, libraries map[string]string, client *apiclient.ApiClient, userId string) (string, error) {
	env, err := NewLocalScriptlingEnv(argv, libraries, client, userId)
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




