package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// registerPluginLoopbackCallLibrary adds knot.plugin to server-side
// environments (user-created MCP tools, event sinks): the loopback twin
// of registerPluginCallLibrary. The two share one call contract —
// call(plugin, handler, params?, method="GET"), name validation,
// declared-handlers-only, the gate enforced for the requesting user — so
// a call behaves the same whichever environment it runs in; only the
// transport differs. This one rides the in-process loopback —
// the same authenticated transport the knot.* libraries use, hitting the
// real web dispatch, so the declared gates apply exactly as for a browser
// fetch. User code never invokes plugin handler code in-process; plugin
// environments keep the in-process bridge (registerPluginCallLibrary)
// within their own trust domain.
func registerPluginLoopbackCallLibrary(env *scriptling.Scriptling, client *apiclient.ApiClient, user *model.User) {
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
				callParams := map[string]any{}
				if len(args) == 3 {
					raw := conversion.ToGo(args[2])
					if m, ok := raw.(map[string]any); ok {
						callParams = m
					} else {
						return errors.NewError("call: params must be a table")
					}
				}
				method := "GET"
				if m := kwargs.Get("method"); m != nil {
					if s, errObj := m.AsString(); errObj == nil {
						method = strings.ToUpper(s)
					}
				}

				transport := client.GetRESTClient()
				if transport == nil {
					return errors.NewError("call: no loopback transport available")
				}

				path := "/plugins/" + pluginName + "/" + handler
				var result any
				var code int
				var err error
				switch method {
				case "GET":
					if len(callParams) > 0 {
						query := url.Values{}
						for key, value := range callParams {
							query.Set(key, fmt.Sprintf("%v", value))
						}
						path += "?" + query.Encode()
					}
					code, err = transport.Get(ctx, path, &result)
				case "POST":
					code, err = transport.PostJSON(ctx, path, callParams, &result, 0)
				default:
					return errors.NewError("call: method must be GET or POST")
				}
				if err != nil && code == 0 {
					return errors.NewError("call: %s", err.Error())
				}
				switch code {
				case 200:
					return conversion.FromGo(result)
				case 403:
					return errors.NewError("call: permission denied for handler %s of plugin %s", handler, pluginName)
				case 404:
					return errors.NewError("call: handler %s of plugin %s is not addressable (no [[tool.knot.handlers]] declaration, or unknown plugin)", handler, pluginName)
				default:
					return errors.NewError("call: handler %s of plugin %s answered HTTP %d", handler, pluginName, code)
				}
			},
			HelpText: "call(plugin, handler, params?, method='GET') - Call a plugin's declared handler as the requesting user, over the authenticated loopback.",
		},
	}, nil, "Calls to declared plugin handlers over the authenticated loopback, as the requesting user.")

	env.RegisterLibrary(lib)
}
