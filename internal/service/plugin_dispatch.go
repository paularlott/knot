package service

import (
	"context"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/scriptling/conversion"
)

// DispatchPluginHandler runs one plugin handler function as the requesting
// user and returns its Go value: the shared core of plugin field handler
// calls. The environment comes from the per-(plugin, user) pool — isolation
// follows the trust boundary — with per-request state (params) set here.
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
	result, err := env.CallFunctionWithContext(ctx, handler)
	if err != nil {
		return nil, err
	}
	return conversion.ToGo(result), nil
}
