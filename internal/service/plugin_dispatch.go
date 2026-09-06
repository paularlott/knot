package service

import (
	"context"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/scriptling/conversion"
)

// DispatchPluginHandler runs one plugin handler function in a fresh
// run-as-user environment and returns its Go value: the shared core of
// plugin page dispatch and plugin field handler calls. The environment is
// discarded with the call - plugin code runs per request and only per
// request.
func DispatchPluginHandler(ctx context.Context, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin, handler string, params map[string]any) (any, error) {
	env, err := NewPluginScriptlingEnv(client, user, plugin)
	if err != nil {
		return nil, err
	}
	if _, err := env.EvalWithContext(ctx, plugin.EntrySource); err != nil {
		return nil, err
	}
	if err := env.SetObjectVar("params", conversion.FromGo(params)); err != nil {
		return nil, err
	}
	result, err := env.CallFunctionWithContext(ctx, handler)
	if err != nil {
		return nil, err
	}
	return conversion.ToGo(result), nil
}
