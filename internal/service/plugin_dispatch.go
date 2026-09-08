package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/conversion"
	scriptlingmcp "github.com/paularlott/scriptling/extlibs/mcp"
	"github.com/paularlott/scriptling/object"
)

// RequestObject builds the `request` argument every plugin handler receives
// as its single parameter: the call's method, path, params dict, and an
// inert snapshot of the requesting user. request["user"] is plain,
// serializable data — a dict of id, name, is_admin, groups, permissions
// (stable snake_case keys) and plugin_permissions (qualified grants) — so a
// handler can branch on who is calling, and the same request dict crosses
// the wire unchanged to a binary peer as its function argument.
//
// request["user"] is data, not authority: it carries no methods and makes no
// round trip. A handler that needs to ACT as the user, or wants the
// authoritative permission check, uses knot.identity.user() (a real User
// object with has_permission / in_group) and the knot.* libraries, which
// round-trip over the gated loopback. Browser fetches carry the real method
// and URL; knot.plugin.call carries its method (GET default) with the
// handler's plugin-root URL; MCP tool execution carries "CALL".
func RequestObject(method, path string, params map[string]any, user *model.User) object.Object {
	req := &object.Dict{Pairs: map[string]object.DictPair{}}
	set := func(key string, value object.Object) {
		req.Pairs[object.DictKey(object.NewString(key))] = object.DictPair{Key: object.NewString(key), Value: value}
	}
	set("method", object.NewString(method))
	set("path", object.NewString(path))
	set("params", conversion.FromGo(params))
	set("user", userDataObject(user))
	return req
}

// userDataObject builds the inert request["user"] snapshot: a plain dict,
// serializable across the plugin protocol to a binary peer. It mirrors the
// fields of the knot.identity User object but carries no methods — identity
// as data, not as authority.
func userDataObject(user *model.User) object.Object {
	if user == nil {
		return &object.Dict{Pairs: map[string]object.DictPair{}}
	}
	strList := func(items []string) any {
		out := make([]any, 0, len(items))
		for _, s := range items {
			out = append(out, s)
		}
		return out
	}
	return conversion.FromGo(map[string]any{
		"id":                 user.Id,
		"name":               user.Username,
		"is_admin":           user.IsAdmin(),
		"groups":             strList(user.Groups),
		"permissions":        strList(user.GrantedPermissionKeys()),
		"plugin_permissions": strList(user.GrantedPluginPermissions()),
	})
}

// QualifiedHandler maps a declared handler reference to the addressable name
// dispatch calls. Handlers live under a plugin.<ns> namespace: a pure-script
// plugin's main.py is registered as plugin.<ScriptNamespace(folder)>, and a
// peer plugin's peer exports under its handshake name. So a bare "status" or
// "mod.fn" declaration resolves against the plugin's default namespace —
// its ScriptNamespace when it has a main.py, else its sole peer's handshake
// name — as "plugin.<ns>.status" / "plugin.<ns>.mod.fn". A reference already
// rooted at plugin.* (a peer plugin naming a specific peer as
// "plugin.demolib.status", or cross-plugin composition addressing another
// plugin) is used verbatim.
func QualifiedHandler(plugin *plugins.Plugin, handler string) string {
	if strings.HasPrefix(handler, "plugin.") {
		return handler
	}
	return "plugin." + plugin.DefaultNamespace() + "." + handler
}

// acquirePluginEnv leases a pooled plugin environment for one dispatch and
// builds the per-request `request` argument every handler path shares. It
// returns the release function the caller must defer plus the request
// object to pass as the handler's argument.
func acquirePluginEnv(ctx context.Context, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin, handler string, params map[string]any, method string) (*scriptling.Scriptling, object.Object, func(), error) {
	env, err := AcquirePluginEnv(ctx, client, user, plugin)
	if err != nil {
		return nil, nil, nil, err
	}
	release := func() { ReleasePluginEnv(env, plugin) }
	request := RequestObject(method, fmt.Sprintf("/plugins/%s/%s", plugin.Name, handler), params, user)
	return env, request, release, nil
}

// DispatchPluginMCPTool runs a plugin handler as an MCP tool: the tool's
// parameters are bound both as the handler's `params` dict and as the MCP
// tool context (__mcp_params), so scriptling.mcp.tool accessors and the
// return_* functions work exactly as they do in script tools. A plain
// return value comes back as result; return_string/return_object surface
// as response (with exit code 0), return_error as a non-zero exit code.
func DispatchPluginMCPTool(ctx context.Context, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin, handler string, params map[string]any) (result any, response string, exitCode int, err error) {
	env, request, release, err := acquirePluginEnv(ctx, client, user, plugin, handler, params, "CALL")
	if err != nil {
		return nil, "", 0, err
	}
	defer release()

	paramsDict := &object.Dict{Pairs: make(map[string]object.DictPair)}
	for key, value := range params {
		paramsDict.SetByString(key, conversion.FromGo(value))
	}
	// The scriptling.mcp.tool accessors read __mcp_params from the global
	// scope of the environment the handler runs in. A plugin handler runs
	// as a function of the plugin.<ns> library module, so bind the params in
	// that module's env; keep the main-env binding too for a top-level
	// handler and for scriptling.mcp.tool internals resolved there.
	qualified := QualifiedHandler(plugin, handler)
	if err := env.SetObjectVar(scriptlingmcp.MCPParamsVarName, paramsDict); err != nil {
		return nil, "", 0, err
	}
	if handlerEnv := handlerGlobalEnv(env, qualified); handlerEnv != nil {
		handlerEnv.Set(scriptlingmcp.MCPParamsVarName, paramsDict)
	}

	res, callErr := env.CallFunctionWithContext(ctx, qualified, request)
	// scriptling.mcp.tool.return_* set __mcp_response via SetGlobal, which
	// lands in the environment the handler ran in. Because a plugin
	// handler runs as a function of the plugin.<ns> library module, that is
	// the module's own env, not the interpreter's main env — so read the
	// response from the handler function's closure env, falling back to the
	// main env for a handler that happens to live at top level.
	if response == "" {
		if str := mcpResponseFromHandler(env, qualified); str != "" {
			response = str
		}
	}
	if response == "" {
		if resObj, getErr := env.GetVarAsObject(scriptlingmcp.MCPResponseVarName); getErr == nil {
			if str, ok := resObj.(*object.String); ok {
				response = str.StringValue()
			}
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

// mcpResponseFromHandler reads the __mcp_response variable from the
// environment the handler ran in. A plugin handler is a function of the
// plugin.<ns> library module, and scriptling.mcp.tool.return_* set the
// response in that module's global scope (SetGlobal on the call-time env),
// which the interpreter's main env does not see. Resolving the handler to
// its function object exposes its closure env, whose global carries the
// response. Returns "" when the handler set none or could not be resolved.
func mcpResponseFromHandler(env *scriptling.Scriptling, qualified string) string {
	handlerEnv := handlerGlobalEnv(env, qualified)
	if handlerEnv == nil {
		return ""
	}
	if obj, ok := handlerEnv.Get(scriptlingmcp.MCPResponseVarName); ok {
		if str, ok := obj.(*object.String); ok {
			return str.StringValue()
		}
	}
	return ""
}

// handlerGlobalEnv returns the global scope of the environment a handler
// runs in: a plugin handler is a function of the plugin.<ns> library module,
// so its closure env's global is where scriptling.mcp.tool reads __mcp_params
// and writes __mcp_response. Returns nil when the handler is not a plain
// scriptling function (e.g. a peer proxy — peer MCP tools do not use the
// in-env param/response vars).
func handlerGlobalEnv(env *scriptling.Scriptling, qualified string) *object.Environment {
	fn := resolveHandlerFunction(env, qualified)
	if fn == nil || fn.Env == nil {
		return nil
	}
	return fn.Env.GetGlobal()
}

// resolveHandlerFunction resolves a dotted handler name (plugin.<ns>.<fn>,
// optionally plugin.<ns>.<mod>.<fn>) to its scriptling function object, or
// nil when it is not a plain function (e.g. a peer proxy, which does not use
// the in-env response var). Traverses the plugin namespace dict the same way
// CallFunctionWithContext does.
func resolveHandlerFunction(env *scriptling.Scriptling, qualified string) *object.Function {
	parts := strings.Split(qualified, ".")
	if len(parts) < 2 {
		return nil
	}
	cur, err := env.GetVarAsObject(parts[0])
	if err != nil {
		return nil
	}
	for _, part := range parts[1:] {
		dict, ok := cur.(*object.Dict)
		if !ok {
			return nil
		}
		pair, ok := dict.GetByString(part)
		if !ok {
			return nil
		}
		cur = pair.Value
	}
	fn, _ := cur.(*object.Function)
	return fn
}

// DispatchPluginHandler runs one plugin handler function as the requesting
// user and returns its Go value: the shared core of plugin field handler
// and MCP tool calls. The environment comes from the per-plugin pool —
// isolation follows the trust boundary — with per-request state (params,
// request) bound by acquirePluginEnv.
func DispatchPluginHandler(ctx context.Context, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin, handler string, params map[string]any) (any, error) {
	return DispatchPluginHandlerWithMethod(ctx, client, user, plugin, handler, params, "GET")
}

// DispatchPluginHandlerWithMethod is DispatchPluginHandler with the
// request method the handler sees — knot.plugin.call's method kwarg rides
// here (GET default, matching the loopback transport's real request).
func DispatchPluginHandlerWithMethod(ctx context.Context, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin, handler string, params map[string]any, method string) (any, error) {
	env, request, release, err := acquirePluginEnv(ctx, client, user, plugin, handler, params, method)
	if err != nil {
		return nil, err
	}
	defer release()

	result, err := env.CallFunctionWithContext(ctx, QualifiedHandler(plugin, handler), request)
	if err != nil {
		return nil, err
	}
	return conversion.ToGo(result), nil
}
