package service

import (
	"context"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// registerPluginCallLibrary adds knot.plugin to plugin handler
// environments (plugin A composing plugin B, same trust domain): the
// in-process twin of registerPluginLoopbackCallLibrary. The two share one
// call contract — call(plugin, handler, params?, method="GET"), name
// validation, declared-handlers-only, the gate enforced for the
// requesting user — so a call behaves the same whichever environment it
// runs in; only the transport differs (here the dispatch is direct, there
// it rides the authenticated loopback through the web dispatch).
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
				if strings.ContainsAny(pluginName, "/") || strings.ContainsAny(handler, "/?&#") {
					return errors.NewError("call: invalid plugin or handler name")
				}
				method := "GET"
				if m := kwargs.Get("method"); m != nil {
					if ms, errObj := m.AsString(); errObj == nil {
						method = strings.ToUpper(ms)
						if method != "GET" && method != "POST" {
							return errors.NewError("call: method must be GET or POST")
						}
					}
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
				if !user.PassesPluginGate(decl.Permission) {
					return errors.NewError("call: permission denied for handler %s of plugin %s", handler, pluginName)
				}

				callClient := client
				if callClient == nil {
					callClient = apiclient.NewMuxClient(user)
				}
				result, err := DispatchPluginHandlerWithMethod(ctx, callClient, user, plugin, handler, callParams, method)
				if err != nil {
					return errors.NewError("call: %s", err.Error())
				}
				return conversion.FromGo(result)
			},
			HelpText: "call(plugin, handler, params?, method='GET') - Call a plugin's declared handler as the requesting user.",
		},
	}, nil, "In-process calls to declared plugin handlers, as the requesting user.")

	env.RegisterLibrary(lib)
}
