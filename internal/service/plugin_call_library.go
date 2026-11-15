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
				pluginName, handler, callParams, method, errObj := pluginCallArgs(kwargs, args)
				if errObj != nil {
					return errObj
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

// pluginCallArgs parses call(plugin, handler, params=None, method="GET")
// for both knot.plugin twins: params and method may be given positionally
// or by keyword, and None means the default.
func pluginCallArgs(kwargs object.Kwargs, args []object.Object) (pluginName, handler string, params map[string]any, method string, errObj object.Object) {
	arg := func(pos int, name string) object.Object {
		v := kwargs.Get(name)
		if v == nil && len(args) > pos {
			v = args[pos]
		}
		if _, isNull := v.(*object.Null); isNull {
			return nil
		}
		return v
	}
	if len(args) > 4 {
		return "", "", nil, "", errors.NewError("call: requires (plugin, handler[, params[, method]])")
	}
	for _, k := range kwargs.Keys() {
		switch k {
		case "plugin", "handler", "params", "method":
		default:
			return "", "", nil, "", errors.NewError("call: unexpected keyword argument %s", k)
		}
	}
	p, h := arg(0, "plugin"), arg(1, "handler")
	if p == nil || h == nil {
		return "", "", nil, "", errors.NewError("call: requires (plugin, handler[, params[, method]])")
	}
	var e object.Object
	if pluginName, e = p.AsString(); e != nil {
		return "", "", nil, "", errors.NewError("call: plugin must be a string")
	}
	if handler, e = h.AsString(); e != nil {
		return "", "", nil, "", errors.NewError("call: handler must be a string")
	}
	if strings.ContainsAny(pluginName, "/") || strings.ContainsAny(handler, "/?&#") {
		return "", "", nil, "", errors.NewError("call: invalid plugin or handler name")
	}
	method = "GET"
	if m := arg(3, "method"); m != nil {
		ms, e := m.AsString()
		if e != nil {
			return "", "", nil, "", errors.NewError("call: method must be GET or POST")
		}
		method = strings.ToUpper(ms)
		if method != "GET" && method != "POST" {
			return "", "", nil, "", errors.NewError("call: method must be GET or POST")
		}
	}
	params = map[string]any{}
	if raw := arg(2, "params"); raw != nil {
		m, ok := conversion.ToGo(raw).(map[string]any)
		if !ok {
			return "", "", nil, "", errors.NewError("call: params must be a table")
		}
		params = m
	}
	return pluginName, handler, params, method, nil
}
