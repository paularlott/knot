package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/scriptling"
)

// pluginEnvPool keeps warm plugin environments, one free list per plugin.
// The reusable, user-independent part of an env — interpreter, ~40 library
// registrations, the plugin's binary peers, jailed paths — survives across
// leases; everything user-visible does not:
//
//   - Reset() on acquire clears the entire binding store: module state,
//     imports, cached data — any residue from the previous user's run.
//   - bindPluginUserIdentity() then attaches the requesting user's knot.*
//     transports, ai_client and loader. Identity is an object on the env,
//     set for the lease, replaced on the next.
//   - The entry source is re-evaluated per lease — cheap, because
//     scriptling caches parsed programs process-wide by source.
//
// So every dispatch starts from a clean module, running entirely as its own
// user, while paying neither the interpreter construction nor the library
// registration that a fresh env costs.
type pluginEnvPool struct {
	mu sync.Mutex
	// idle holds released envs per plugin, ready for Reset+rebind. Entries
	// carry their release time for expiry.
	idle      map[*plugins.Plugin][]pooledPluginEnv
	idleTotal int
	// leases counts dispatches served from a pooled env; builds counts cold
	// starts. Guarded by mu, for tests and a little observability.
	leases, builds int
}

type pooledPluginEnv struct {
	env        *scriptling.Scriptling
	releasedAt time.Time
}

const (
	// pluginEnvIdleTTL bounds how long an unused interpreter is held; it is
	// memory hygiene only — entries are user-agnostic and revalidated by
	// plugin identity (the map key), so a reloaded plugin never reuses an
	// old env.
	pluginEnvIdleTTL = 90 * time.Second
	// pluginEnvMaxIdle bounds memory across all plugins' free lists.
	pluginEnvMaxIdle = 512
)

var pluginEnvs = &pluginEnvPool{idle: map[*plugins.Plugin][]pooledPluginEnv{}}

// AcquirePluginEnv returns an environment for dispatching the plugin's
// handlers as the user: a pooled env Reset and rebound to the user (with
// the entry evaluated), or a freshly built one when the pool is empty. The
// caller MUST call ReleasePluginEnv when done — an acquired env is
// exclusively leased until released.
func AcquirePluginEnv(ctx context.Context, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin) (*scriptling.Scriptling, error) {
	pluginEnvs.mu.Lock()
	list := pluginEnvs.idle[plugin]
	// Copy the entry out by VALUE and shrink the slice. Taking &list[n-1]
	// would hand back a pointer into the slice's backing array — the exact
	// slot a later ReleasePluginEnv reuses on append — so the released env
	// and the next lease would alias one slot and race. A value copy severs
	// that; the env pointer inside is owned exclusively by this lease.
	var pooled pooledPluginEnv
	var havePooled bool
	if n := len(list); n > 0 {
		pooled = list[n-1]
		list[n-1] = pooledPluginEnv{} // drop the reference so it can't alias
		pluginEnvs.idle[plugin] = list[:n-1]
		pluginEnvs.idleTotal--
		havePooled = true
	}
	if havePooled && time.Now().After(pooled.releasedAt.Add(pluginEnvIdleTTL)) {
		havePooled = false // expired — drop it and build fresh
	}
	if havePooled {
		pluginEnvs.leases++
	} else {
		pluginEnvs.builds++
	}
	pluginEnvs.mu.Unlock()

	if !havePooled {
		env, err := NewPluginScriptlingEnv(client, user, plugin)
		if err != nil {
			return nil, err
		}
		if err := importPluginNamespace(ctx, env, plugin); err != nil {
			return nil, err
		}
		env.EnableOutputCapture()
		return env, nil
	}

	// Warm lease: wipe the previous run's state, attach this user, and
	// re-materialize the plugin's own namespace. Reset keeps the registered
	// libraries and peers — that reuse is the point — but clears imported
	// bindings, so plugin.<name> must be imported again.
	pooled.env.Reset()
	if err := bindPluginUserIdentity(pooled.env, client, user, plugin); err != nil {
		return nil, err
	}
	if err := importPluginNamespace(ctx, pooled.env, plugin); err != nil {
		return nil, err
	}
	pooled.env.EnableOutputCapture()
	return pooled.env, nil
}

// importPluginNamespace materializes the plugin.<x> bindings this plugin's
// declared handlers are addressed through. Registration (buildPluginEnv)
// puts a library in the env's table but does not bind the dotted name;
// importing evaluates it and binds plugin.<x> so plugin.<x>.<fn> resolves.
// A pure-script plugin's namespace is its main.py (ScriptNamespace); a peer
// plugin's are its peers' handshake names. Reset clears imported bindings,
// so this runs on every lease, not just cold builds.
//
// The plugin's own main.py is re-registered before import so its cached
// module store is dropped and the module body re-evaluates every lease:
// each dispatch starts from a clean module, and one user's module-level
// state never leaks to the next. Peer namespaces are process-backed (no
// in-env module state) and composed plugins are imported by handler code as
// needed, matching the pre-pool behaviour.
func importPluginNamespace(ctx context.Context, env *scriptling.Scriptling, plugin *plugins.Plugin) error {
	if plugin.EntrySource != "" {
		// Fresh registration clears any cached evaluated store, so the
		// import below re-runs the module body for this lease.
		if err := env.RegisterScriptLibrary("plugin."+plugin.ScriptNamespace, plugin.EntrySource); err != nil {
			return fmt.Errorf("register plugin.%s: %w", plugin.ScriptNamespace, err)
		}
	}
	for _, ns := range plugin.Namespaces {
		if _, err := env.EvalWithContext(ctx, "import plugin."+ns+"\n"); err != nil {
			return fmt.Errorf("import plugin.%s: %w", ns, err)
		}
	}
	return nil
}

// ReleasePluginEnv returns a dispatched env to its plugin's free list.
// Output printed by handlers is drained so an env's capture buffer cannot
// grow across leases; overflowing or stale lists are trimmed.
func ReleasePluginEnv(env *scriptling.Scriptling, plugin *plugins.Plugin) {
	_ = env.GetOutput()

	pluginEnvs.mu.Lock()
	defer pluginEnvs.mu.Unlock()
	if pluginEnvs.idleTotal >= pluginEnvMaxIdle {
		now := time.Now()
		for p, list := range pluginEnvs.idle {
			kept := list[:0]
			for _, e := range list {
				if now.Before(e.releasedAt.Add(pluginEnvIdleTTL)) {
					kept = append(kept, e)
				}
			}
			pluginEnvs.idle[p] = kept
			pluginEnvs.idleTotal -= len(list) - len(kept)
			if len(kept) == 0 {
				delete(pluginEnvs.idle, p)
			}
		}
		if pluginEnvs.idleTotal >= pluginEnvMaxIdle {
			return // nothing evictable — don't pool this one
		}
	}
	pluginEnvs.idle[plugin] = append(pluginEnvs.idle[plugin], pooledPluginEnv{env: env, releasedAt: time.Now()})
	pluginEnvs.idleTotal++
}

// PluginEnvPoolStats reports warm leases and cold builds (tests,
// observability).
func PluginEnvPoolStats() (leases, builds int) {
	pluginEnvs.mu.Lock()
	defer pluginEnvs.mu.Unlock()
	return pluginEnvs.leases, pluginEnvs.builds
}

// ResetPluginEnvPoolForTest drops every pooled env.
func ResetPluginEnvPoolForTest() {
	pluginEnvs.mu.Lock()
	defer pluginEnvs.mu.Unlock()
	pluginEnvs.idle = map[*plugins.Plugin][]pooledPluginEnv{}
	pluginEnvs.idleTotal = 0
	pluginEnvs.leases, pluginEnvs.builds = 0, 0
}
