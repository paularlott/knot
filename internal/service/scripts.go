package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/scriptling"
)

func ExecuteScriptWithMCP(script *model.Script, mcpParams map[string]string, user *model.User, client *apiclient.ApiClient) (string, error) {
	timeout := time.Duration(script.Timeout) * time.Second
	if script.Timeout == 0 {
		timeout = 300 * time.Second // 5 minutes to allow for AI operations with tool calling
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ctx = context.WithValue(ctx, "user", user)

	env, err := NewMCPScriptlingEnv(client, mcpParams, user)
	if err != nil {
		return "", fmt.Errorf("failed to create scriptling environment: %v", err)
	}

	// Register MCP library with parameters and tool access
	mcpLib := scriptling.GetMCPLibrary(mcpParams, GetOpenAIClient())
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

func ExecuteScriptLocally(script *model.Script, args []string) (string, error) {
	if script.ScriptType == "tool" {
		return "", fmt.Errorf("tool scripts can only be executed via MCP")
	}

	timeout := time.Duration(script.Timeout) * time.Second
	if script.Timeout == 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	env, err := NewRemoteScriptlingEnv(args, nil, "")
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
