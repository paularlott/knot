package service

import (
	"context"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// registerPluginCallLibrary adds knot.plugin to environments that may call
// plugin handlers in-process: server-side script envs (built-in and user
// MCP tools) and plugin handler envs themselves (plugin A composing plugin
// B). call(plugin, handler, params?) dispatches the target's *declared*
// handler — the same [[tool.knot.handlers]] contract the browser's
// pluginFetch uses — as the requesting user, with the declaration's gate
// enforced. Undeclared handlers are not addressable.
func registerPluginCallLibrary(env *scriptling.Scriptling, client *apiclient.ApiClient, user *model.User) {
	if user == nil {
		return
	}
	lib := object.NewLibrary("knot.plugin", map[string]*object.Builtin{
		"call": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return errors.NewError("call: requires (plugin, handler[, params])")
				}
				pluginName, errObj := args[0].AsString()
				if errObj != nil {
					return errors.NewError("call: plugin must be a string")
				}
				handler, errObj := args[1].AsString()
				if errObj != nil {
					return errors.NewError("call: handler must be a string")
				}
				callParams := map[string]any{}
				if len(args) == 3 {
					raw := conversion.ToGo(args[2])
					if m, ok := raw.(map[string]any); ok {
						callParams = m
					} else {
						return errors.NewError("call: params must be a table")
					}
				}

				registry := plugins.GetRegistry()
				if registry == nil {
					return errors.NewError("call: no plugins are loaded")
				}
				plugin := registry.ByName(pluginName)
				if plugin == nil {
					return errors.NewError("call: unknown plugin %s", pluginName)
				}
				decl := plugin.HandlerDecl(handler)
				if decl == nil {
					return errors.NewError("call: handler %s of plugin %s is not addressable (no [[tool.knot.handlers]] declaration)", handler, pluginName)
				}
				if decl.Permission != "" && !user.HasPluginPermission(decl.Permission) {
					return errors.NewError("call: permission denied for handler %s of plugin %s", handler, pluginName)
				}
				if len(decl.Groups) > 0 && !user.HasAnyGroup(&decl.Groups) {
					return errors.NewError("call: permission denied for handler %s of plugin %s", handler, pluginName)
				}

				callClient := client
				if callClient == nil {
					callClient = apiclient.NewMuxClient(user)
				}
				result, err := DispatchPluginHandler(ctx, callClient, user, plugin, handler, callParams)
				if err != nil {
					return errors.NewError("call: %s", err.Error())
				}
				return conversion.FromGo(result)
			},
			HelpText: "call(plugin, handler, params?) - Call a plugin's declared handler as the requesting user.",
		},
	}, nil, "In-process calls to declared plugin handlers, as the requesting user.")

	env.RegisterLibrary(lib)
}
