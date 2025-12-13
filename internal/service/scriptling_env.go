package service

import (
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
