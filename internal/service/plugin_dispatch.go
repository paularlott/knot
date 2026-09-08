package service

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/conversion"
	scriptlingmcp "github.com/paularlott/scriptling/extlibs/mcp"
	"github.com/paularlott/scriptling/object"
)

// RequestObject builds the `request` object every plugin handler dispatch
// receives: the call's method and path. Browser fetches carry the real
// method and URL; in-process dispatch (MCP tools, knot.plugin.call inside
// plugin envs) carries method "CALL" and the handler's plugin-root URL,
// so request.path is meaningful on every path a handler is reached by.
func RequestObject(method, path string) map[string]any {
	return map[string]any{"method": method, "path": path}
}

// acquirePluginEnv leases a pooled plugin environment for one dispatch and
// binds the per-request state every handler path shares: the params dict,
// the request object, and the requesting user. It returns the release
// function the caller must defer.
func acquirePluginEnv(ctx context.Context, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin, handler string, params map[string]any) (*scriptling.Scriptling, func(), error) {
	env, err := AcquirePluginEnv(ctx, client, user, plugin)
	if err != nil {
		return nil, nil, err
	}
	release := func() { ReleasePluginEnv(env, plugin) }
	if err := env.SetObjectVar("params", conversion.FromGo(params)); err != nil {
		release()
		return nil, nil, err
	}
	if err := env.SetObjectVar("request", conversion.FromGo(RequestObject("CALL", fmt.Sprintf("/plugins/%s/%s", plugin.Name, handler)))); err != nil {
		release()
		return nil, nil, err
	}
	if err := env.SetObjectVar("user", NewUserObject(user)); err != nil {
		release()
		return nil, nil, err
	}
	return env, release, nil
}

// DispatchPluginMCPTool runs a plugin handler as an MCP tool: the tool's
// parameters are bound both as the handler's `params` dict and as the MCP
// tool context (__mcp_params), so scriptling.mcp.tool accessors and the
// return_* functions work exactly as they do in script tools. A plain
// return value comes back as result; return_string/return_object surface
// as response (with exit code 0), return_error as a non-zero exit code.
func DispatchPluginMCPTool(ctx context.Context, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin, handler string, params map[string]any) (result any, response string, exitCode int, err error) {
	env, release, err := acquirePluginEnv(ctx, client, user, plugin, handler, params)
	if err != nil {
		return nil, "", 0, err
	}
	defer release()

	paramsDict := &object.Dict{Pairs: make(map[string]object.DictPair)}
	for key, value := range params {
		paramsDict.SetByString(key, conversion.FromGo(value))
	}
	if err := env.SetObjectVar(scriptlingmcp.MCPParamsVarName, paramsDict); err != nil {
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
// request) bound by acquirePluginEnv.
func DispatchPluginHandler(ctx context.Context, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin, handler string, params map[string]any) (any, error) {
	env, release, err := acquirePluginEnv(ctx, client, user, plugin, handler, params)
	if err != nil {
		return nil, err
	}
	defer release()

	result, err := env.CallFunctionWithContext(ctx, handler)
	if err != nil {
		return nil, err
	}
	return conversion.ToGo(result), nil
}
