package service

import (
	"context"
	"sync"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
	pluginpkg "github.com/paularlott/scriptling/plugin"
)

// agentPluginManager holds the scriptling plugins loaded for the agent's
// scriptling environments (--plugin / --plugin-dir), mirroring the standalone
// scriptling CLI. It is built once at agent startup and registered into every
// agent env by registerAgentLibraries. nil until LoadAgentPlugins runs (and
// when no plugins are configured), so the default agent carries no external
// plugins and stays lean.
var (
	agentPluginMu      sync.RWMutex
	agentPluginManager *pluginpkg.Manager
)

// LoadAgentPlugins spawns the agent's scriptling plugins and holds the manager
// for the agent env to register. It mirrors the scriptling CLI: explicit
// --plugin executables load first, in order (a binary's identity is its
// resolved path, so the same binary found again via --plugin-dir is a no-op),
// then each --plugin-dir is scanned. Both empty is a no-op. A plugin that
// fails to start is a hard error, matching the CLI — a space that asked for a
// driver should fail loudly rather than run without it.
func LoadAgentPlugins(ctx context.Context, plugins, dirs []string) error {
	if len(plugins) == 0 && len(dirs) == 0 {
		return nil
	}

	logger := log.WithGroup("agent-plugins")
	manager := pluginpkg.NewManager(loggerForAgentPlugins(), func(name string, err error) {
		logger.Error("plugin process exited", "plugin", name, "error", err)
	})

	specs := make([]pluginpkg.PluginSpec, 0, len(plugins))
	for _, path := range plugins {
		specs = append(specs, pluginpkg.PluginSpec{Path: path})
	}
	if len(specs) > 0 {
		if err := manager.LoadPlugins(ctx, specs); err != nil {
			_ = manager.Close()
			return err
		}
	}

	for _, dir := range dirs {
		manager.AddDir(dir)
	}
	if err := manager.Load(ctx); err != nil {
		_ = manager.Close()
		return err
	}
	for _, warning := range manager.Warnings() {
		logger.Warn("plugin load warning", "warning", warning)
	}
	for _, md := range manager.List() {
		logger.Info("plugin loaded", "plugin", md.Name, "version", md.Version)
	}

	agentPluginMu.Lock()
	old := agentPluginManager
	agentPluginManager = manager
	agentPluginMu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

// getAgentPluginManager returns the loaded agent plugin manager, or nil.
func getAgentPluginManager() *pluginpkg.Manager {
	agentPluginMu.RLock()
	defer agentPluginMu.RUnlock()
	return agentPluginManager
}

// CloseAgentPlugins shuts down every spawned agent plugin process. Called on
// agent shutdown.
func CloseAgentPlugins() {
	agentPluginMu.Lock()
	manager := agentPluginManager
	agentPluginManager = nil
	agentPluginMu.Unlock()
	if manager != nil {
		_ = manager.Close()
	}
}

// loggerForAgentPlugins is the scriptling-side logger the plugin manager uses
// for plugin stderr/log forwarding.
func loggerForAgentPlugins() logger.Logger {
	return logger.NewNullLogger()
}
