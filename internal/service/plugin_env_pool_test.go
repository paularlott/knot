package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
)

// pooledFixture loads a one-plugin registry whose handler reads per-request
// params and counts module-level state — the counter must restart on every
// lease, pooled or not, proving Reset clears the previous run's residue.
func pooledFixture(t *testing.T) *plugins.Plugin {
	t.Helper()
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "pooled")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `# /// script
# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0"
# ///
dispatches = 0


def value(request):
    global dispatches
    params = request["params"]
    dispatches = dispatches + 1
    return {"n": dispatches, "word": params.get("word", "")}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := plugins.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	plugins.SetRegistry(registry)
	t.Cleanup(func() {
		registry.Close()
		plugins.SetRegistry(nil)
	})
	return registry.ByName("pooled")
}

// TestPluginEnvPoolLease pins the shared-pool contract: envs are pooled per
// plugin and shared across users; every lease starts from a clean module
// (the previous run's state — including another user's — is gone), identity
// is rebound per lease, and per-request params are always fresh.
func TestPluginEnvPoolLease(t *testing.T) {
	model.SetRoleCache(nil)
	ResetPluginEnvPoolForTest()
	plugin := pooledFixture(t)
	user := &model.User{Username: "tester", Id: "u-1"}
	other := &model.User{Username: "other", Id: "u-2"}

	ctx := context.Background()
	n := func(m map[string]any) string { return fmt.Sprintf("%v", m["n"]) }
	call := func(u *model.User, word string) map[string]any {
		t.Helper()
		got, err := DispatchPluginHandler(ctx, apiclient.NewMuxClient(u), u, plugin, "value", map[string]any{"word": word})
		if err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		return got.(map[string]any)
	}

	first := call(user, "hi")
	if n(first) != "1" || first["word"] != "hi" {
		t.Fatalf("first dispatch = %v", first)
	}
	second := call(user, "there")
	if n(second) != "1" {
		t.Fatalf("second dispatch n = %v, want 1 (every lease starts a clean module)", second["n"])
	}
	if second["word"] != "there" {
		t.Fatalf("second dispatch word = %v, want fresh params", second["word"])
	}

	leases, builds := PluginEnvPoolStats()
	if builds != 1 || leases != 1 {
		t.Fatalf("pool stats: builds = %d, leases = %d, want 1/1", builds, leases)
	}

	// Another user leases the same pooled env: still a clean module, never
	// the previous user's residue, and no second build.
	third := call(other, "theirs")
	if n(third) != "1" || third["word"] != "theirs" {
		t.Fatalf("other user dispatch = %v, want a clean module with their params", third)
	}
	leases, builds = PluginEnvPoolStats()
	if builds != 1 || leases != 2 {
		t.Fatalf("pool stats after other user: builds = %d, leases = %d, want 1/2", builds, leases)
	}
}
