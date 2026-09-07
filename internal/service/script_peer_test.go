package service

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
)

// TestScriptPeerInProcess pins scriptling-authored peers: a .py in the
// plugin's peers/ folder loads in-process (no CLI, no subprocess) and its
// public surface — functions, classes with state — is importable as
// plugin.<name>, with the consuming plugin's dependency validated against
// the peer's [tool.knot.peer] version.
func TestScriptPeerInProcess(t *testing.T) {
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})
	model.SetRoleCache(nil)

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plug")
	peersDir := filepath.Join(pluginDir, "peers")
	if err := os.MkdirAll(peersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	entry := `# /// script
# requires-scriptling = ">=0.24"
# dependencies = [
#   "plugin.calc via calc >= 1.0.0",
# ]
#
# [tool.knot]
# version = "1.0"
#
# [[tool.knot.handlers]]
# handler = "add_up"
# ///
def add_up():
    import plugin.calc as calc

    c = calc.Counter(4)
    return {"sum": calc.add(2, 3), "first": c.next(), "second": c.next()}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}

	peer := `"""An in-process scriptling peer: public functions, classes and constants."""

MAX = 100


def add(a, b):
    """Add two numbers."""
    return a + b


def _private(a):
    return a


class Counter:
    """A stateful counter."""

    def __init__(self, step):
        self.step = step
        self.n = 0

    def next(self):
        self.n = self.n + self.step
        return self.n
`
	if err := os.WriteFile(filepath.Join(peersDir, "calc.py"), []byte(peer), 0o644); err != nil {
		t.Fatal(err)
	}

	registry, err := plugins.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	plugin := registry.ByName("plug")
	if plugin == nil {
		t.Fatalf("plugin not loaded: %+v", registry.Failed())
	}
	if len(plugin.ScriptPeers) != 1 || plugin.ScriptPeers[0].Name != "calc" || plugin.ScriptPeers[0].Version != "1.0" {
		t.Fatalf("script peers = %+v, want calc 1.0", plugin.ScriptPeers)
	}

	user := &model.User{Username: "tester", Roles: []string{model.RoleAdminUUID}}
	got, err := DispatchPluginHandler(t.Context(), apiclient.NewMuxClient(user), user, plugin, "add_up", map[string]any{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	dict, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("add_up = %#v", got)
	}
	if fmt.Sprintf("%v", dict["sum"]) != "5" || fmt.Sprintf("%v", dict["first"]) != "4" || fmt.Sprintf("%v", dict["second"]) != "8" {
		t.Fatalf("peer results = %v, want sum 5, counter 4 then 8 (native class state)", dict)
	}
}

// TestScriptPeerVersionGate pins dependency validation: a consuming plugin
// requiring a newer peer version than peers/calc.py declares fails to load.
func TestScriptPeerVersionGate(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plug")
	peersDir := filepath.Join(pluginDir, "peers")
	if err := os.MkdirAll(peersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := `# /// script
# requires-scriptling = ">=0.24"
# dependencies = [
#   "plugin.calc via calc >= 2.0.0",
# ]
#
# [tool.knot]
# version = "1.0"
# ///
def add_up():
    return {}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	peer := `# /// script
# [tool.knot.peer]
# version = "1.5"
# ///
def add(a, b):
    return a + b
`
	if err := os.WriteFile(filepath.Join(peersDir, "calc.py"), []byte(peer), 0o644); err != nil {
		t.Fatal(err)
	}

	registry, err := plugins.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	failed := registry.Failed()
	if len(failed) != 1 || !strings.Contains(failed[0].Reason, "calc") {
		t.Fatalf("failed = %+v, want the version mismatch named", failed)
	}
}
