package service

import (
	"sync"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/plugin"
)

var (
	pluginManagerOnce sync.Once
	pluginManager     *plugin.Manager
)

// getPluginManager returns the shared, long-lived plugin manager.
//
// It owns the pooled HTTP transports that are shared across every script
// execution so that connections are reused between runs. The manager holds no
// startup plugins of its own — scripts register HTTP(S) peers at runtime via
// scriptling.plugin.load(). Each execution creates a child scope (see
// registerPluginScope) so that plugins loaded by one user/script cannot leak
// into another user's execution.
func getPluginManager() *plugin.Manager {
	pluginManagerOnce.Do(func() {
		pluginManager = plugin.NewManager(log.GetLogger())
	})
	return pluginManager
}

// ClosePluginManager shuts down the shared plugin manager and releases its
// pooled HTTP transports. It should be called once during server shutdown.
func ClosePluginManager() error {
	if pluginManager != nil {
		return pluginManager.Close()
	}
	return nil
}

// registerPluginScope creates a fresh child scope from the shared plugin
// manager, restricted to the requested transport mode, and registers the
// scriptling.plugin control library (plus any plugins the scope can see) into
// env. The returned value is the scope, which the caller MUST Close (typically
// via defer) once the script has finished executing so that per-execution plugin
// connections are released.
//
// Use plugin.TransportHTTP for server-side environments that must not spawn
// arbitrary local executables (MCP tools, events, health checks), and
// plugin.TransportAll for space-side environments that already have subprocess
// access.
func registerPluginScope(env *scriptling.Scriptling, mode plugin.TransportMode) *plugin.Manager {
	scope := getPluginManager().NewScope(plugin.WithTransport(mode))
	plugin.RegisterLibraries(env, scope)
	return scope
}
