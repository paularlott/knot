package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

func ExecuteScriptInSpace(space *model.Space, script *model.Script, libraries map[string]string, args []string) (string, error) {
	return ExecuteScriptLocally(script, libraries, args)
}

func ExecuteScriptLocally(script *model.Script, libraries map[string]string, args []string) (string, error) {
	timeout := time.Duration(script.Timeout) * time.Second
	if script.Timeout == 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	env, err := NewScriptlingEnv(args, libraries)
	if err != nil {
		return "", fmt.Errorf("failed to create scriptling environment: %v", err)
	}

	result, err := env.EvalWithContext(ctx, script.Content)
	if err != nil {
		return "", fmt.Errorf("script execution failed: %v", err)
	}

	output := env.GetOutput()
	if result != nil && result.Inspect() != "None" {
		if output != "" {
			output += "\n"
		}
		output += result.Inspect()
	}

	return strings.TrimRight(output, "\n"), nil
}
