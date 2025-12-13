package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/scriptling"
)

func ExecuteScriptInSpace(space *model.Space, script *model.Script, libraries map[string]string, args []string) (string, error) {
	return ExecuteScriptLocally(script, libraries, args)
}

func ExecuteScriptWithMCP(script *model.Script, libraries map[string]string, mcpParams map[string]string) (string, error) {
	timeout := time.Duration(script.Timeout) * time.Second
	if script.Timeout == 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	env, err := NewScriptlingEnv(nil, libraries)
	if err != nil {
		return "", fmt.Errorf("failed to create scriptling environment: %v", err)
	}

	// Register MCP library with parameters
	mcpLib := scriptling.GetMCPLibrary(mcpParams)
	env.RegisterLibrary("mcp", mcpLib)

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

func ExecuteScriptLocally(script *model.Script, libraries map[string]string, args []string) (string, error) {
	// Tool scripts can only be executed via MCP
	if script.ScriptType == "tool" {
		return "", fmt.Errorf("tool scripts can only be executed via MCP")
	}

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
