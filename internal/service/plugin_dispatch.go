package service

import (
	"context"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	scriptlingmcp "github.com/paularlott/scriptling/extlibs/mcp"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/object"
)

// RequestObject builds the `request` object every plugin handler dispatch
// receives: the call's method and path.
func RequestObject(method, path string) map[string]any {
	return map[string]any{"method": method, "path": path}
}

// DispatchPluginMCPTool runs a plugin handler as an MCP tool: the tool's
// parameters are bound both as the handler's `params` dict and as the MCP
// tool context (__mcp_params), so scriptling.mcp.tool accessors and the
// return_* functions work exactly as they do in script tools. A plain
// return value comes back as result; return_string/return_object surface
// as response (with exit code 0), return_error as a non-zero exit code.
func DispatchPluginMCPTool(ctx context.Context, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin, handler string, params map[string]any) (result any, response string, exitCode int, err error) {
	env, err := AcquirePluginEnv(ctx, client, user, plugin)
	if err != nil {
		return nil, "", 0, err
	}
	defer ReleasePluginEnv(env, plugin)

	if err := env.SetObjectVar("params", conversion.FromGo(params)); err != nil {
		return nil, "", 0, err
	}
	paramsDict := &object.Dict{Pairs: make(map[string]object.DictPair)}
	for key, value := range params {
		paramsDict.SetByString(key, conversion.FromGo(value))
	}
	if err := env.SetObjectVar(scriptlingmcp.MCPParamsVarName, paramsDict); err != nil {
		return nil, "", 0, err
	}
	if err := env.SetObjectVar("request", conversion.FromGo(RequestObject("CALL", ""))); err != nil {
		return nil, "", 0, err
	}
	if err := env.SetObjectVar("user", NewUserObject(user)); err != nil {
		return nil, "", 0, err
	}

	res, callErr := env.CallFunctionWithContext(ctx, handler)
	if resObj, getErr := env.GetVarAsObject(scriptlingmcp.MCPResponseVarName); getErr == nil {
		if str, ok := resObj.(*object.String); ok {
			response = str.StringValue()
		}
	}
	if exc, ok := object.AsException(res); ok && exc.IsSystemExit() {
		exitCode = exc.GetExitCode()
		if exitCode != 0 && callErr != nil {
			err = callErr
		}
		return nil, response, exitCode, err
	}
	if callErr != nil {
		return nil, response, 0, callErr
	}
	return conversion.ToGo(res), response, 0, nil
}

// DispatchPluginHandler runs one plugin handler function as the requesting
// user and returns its Go value: the shared core of plugin field handler
// and MCP tool calls. The environment comes from the per-plugin pool —
// isolation follows the trust boundary — with per-request state (params,
// request) set here.
func DispatchPluginHandler(ctx context.Context, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin, handler string, params map[string]any) (any, error) {
	// A pooled env Reset and rebound to this user per lease — the same
	// economy the page dispatcher gets.
	env, err := AcquirePluginEnv(ctx, client, user, plugin)
	if err != nil {
		return nil, err
	}
	defer ReleasePluginEnv(env, plugin)
	if err := env.SetObjectVar("params", conversion.FromGo(params)); err != nil {
		return nil, err
	}
	if err := env.SetObjectVar("request", conversion.FromGo(RequestObject("CALL", ""))); err != nil {
		return nil, err
	}
	if err := env.SetObjectVar("user", NewUserObject(user)); err != nil {
		return nil, err
	}
	result, err := env.CallFunctionWithContext(ctx, handler)
	if err != nil {
		return nil, err
	}
	return conversion.ToGo(result), nil
}
