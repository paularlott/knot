package service

import (
	"context"
	"os"

	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/extlibs"
)

// NewScriptlingEnv creates a new scriptling environment with standard libraries registered
func NewScriptlingEnv(argv []string, libraries map[string]string) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	extlibs.RegisterSubprocessLibrary(env)
	env.EnableOutputCapture()

	for name, content := range libraries {
		if err := env.RegisterScriptLibrary(name, content); err != nil {
			return nil, err
		}
	}

	extlibs.RegisterSysLibrary(env, argv)
	return env, nil
}

// NewScriptlingEnvWithDiskLibraries creates a scriptling environment with on-demand library loading from disk
// and pathlib support for agent/desktop use
func NewScriptlingEnvWithDiskLibraries(argv []string, libraries map[string]string) (*scriptling.Scriptling, error) {
	env := scriptling.New()
	extlibs.RegisterSubprocessLibrary(env)
	extlibs.RegisterPathlibLibrary(env, []string{})
	env.EnableOutputCapture()

	for name, content := range libraries {
		if err := env.RegisterScriptLibrary(name, content); err != nil {
			return nil, err
		}
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

// RunScript executes a script with on-demand library loading
func RunScript(ctx context.Context, scriptContent string, argv []string, libraries map[string]string) (string, error) {
	env, err := NewScriptlingEnvWithDiskLibraries(argv, libraries)
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
