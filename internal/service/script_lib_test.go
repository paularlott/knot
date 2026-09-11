package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling/object"
)

// TestScriptLibInProcess pins scriptling-authored libraries: a .py in the
// plugin's libs/ folder loads in-process (no CLI, no subprocess) and its
// public surface — functions, classes with state — is importable as
// plugin.<name>, with the consuming plugin's dependency validated against
// the library's [tool.knot.lib] version.
func TestScriptLibInProcess(t *testing.T) {
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})
	model.SetRoleCache(nil)

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plug")
	libsDir := filepath.Join(pluginDir, "libs")
	if err := os.MkdirAll(libsDir, 0o755); err != nil {
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
def add_up(request):
    import plugin.calc as calc

    c = calc.Counter(4)
    return {"sum": calc.add(2, 3), "first": c.next(), "second": c.next()}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}

	peer := `"""An in-process scriptling library: public functions, classes and constants."""

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
	if err := os.WriteFile(filepath.Join(libsDir, "calc.py"), []byte(peer), 0o644); err != nil {
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
	if len(plugin.Libs) != 1 || plugin.Libs[0].Name != "calc" || plugin.Libs[0].Version != "1.0" {
		t.Fatalf("script libs = %+v, want calc 1.0", plugin.Libs)
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

// TestScriptLibVersionGate pins dependency validation: a consuming plugin
// requiring a newer library version than libs/calc.py declares fails to load.
func TestScriptLibVersionGate(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plug")
	libsDir := filepath.Join(pluginDir, "libs")
	if err := os.MkdirAll(libsDir, 0o755); err != nil {
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
# [tool.knot.lib]
# version = "1.5"
# ///
def add(a, b):
    return a + b
`
	if err := os.WriteFile(filepath.Join(libsDir, "calc.py"), []byte(peer), 0o644); err != nil {
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

// TestUserToolCannotImportPlugin pins the isolation boundary: the plugin
// pool is not attached to the user-tool (MCP) environment, so a user-created
// tool cannot import a plugin's exported library at all — plugin.<name> is
// simply not there. Untrusted code reaches a plugin only through the gated
// loopback (knot.plugin.call), never by importing plugin code in-process.
// This is structural (not attached), not a policy a plugin could forget.
func TestUserToolCannotImportPlugin(t *testing.T) {
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{MCPToolTimeout: 30})
	model.SetRoleCache(nil)
	user := &model.User{Username: "plain", Id: "u-p"}

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "guarded")
	libsDir := filepath.Join(pluginDir, "libs")
	if err := os.MkdirAll(libsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := `# /// script
# requires-scriptling = ">=0.24"
#
# [tool.knot]
# version = "1.0"
# ///
def unused(request):
    return {}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := `def open_add(a, b):
    return a + b
`
	if err := os.WriteFile(filepath.Join(libsDir, "calc.py"), []byte(lib), 0o644); err != nil {
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

	run := func(body string) (string, error) {
		script := &model.Script{Name: "wall_tool", ScriptType: "tool", Active: true, Content: body}
		return ExecuteScriptWithMCP(script, map[string]object.Object{}, user)
	}

	// Importing the plugin's exported library from a user tool must fail:
	// the pool is not attached to this environment.
	if _, err := run("import plugin.calc as calc\nprint(calc.open_add(2, 3))"); err == nil {
		t.Fatal("user tool imported plugin.calc, want the import to fail (pool not attached)")
	}

	// The blessed path is still present: knot.plugin is registered so a
	// user tool can reach a plugin's declared handlers over the gated
	// loopback. (No handler is declared here, so we only assert the library
	// imports — the wall removed the in-process surface, not the loopback.)
	if _, err := run("import knot.plugin\nprint('loopback ok')"); err != nil {
		t.Fatalf("knot.plugin (loopback) should remain available to user tools: %v", err)
	}
}

// TestExampleLibExports runs the shipped demo-scriptling example's
// lib_exports handler end to end: the plugin's scriptling library (its
// constant, functions and Counter class) answers a dispatch, and the
// self-gating export passes for an admin (view_dashboard held by bypass).
func TestExampleLibExports(t *testing.T) {
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{MCPToolTimeout: 30})
	model.SetRoleCache(nil)

	examples := filepath.Join("..", "..", "examples", "plugins")
	if _, err := os.Stat(examples); err != nil {
		t.Skip("examples/plugins not present")
	}
	registry, err := plugins.Load(examples)
	if err != nil {
		t.Fatal(err)
	}
	plugins.SetRegistry(registry)
	t.Cleanup(func() {
		registry.Close()
		plugins.SetRegistry(nil)
	})
	demo := registry.ByName("demo-scriptling")
	if demo == nil {
		t.Fatalf("demo-scriptling not loaded; failed = %+v", registry.Failed())
	}

	admin := &model.User{Username: "admin", Id: "u-a", Roles: []string{model.RoleAdminUUID}}
	got, err := DispatchPluginHandler(context.Background(), apiclient.NewMuxClient(admin), admin, demo, "lib_exports", nil)
	if err != nil {
		t.Fatalf("lib_exports: %v", err)
	}
	enc, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"import plugin.calc", // the lib
		"100",                // MAX constant
		"5",                  // add(2, 3)
		"4, 8 (value 8)",     // Counter(4): stateful class
		"granted for admin",  // the self-gating export
	} {
		if !strings.Contains(string(enc), want) {
			t.Errorf("lib_exports missing %q: %s", want, enc)
		}
	}
}

// TestGoPeerNotInUserTool pins the wall for binary peers too: a user tool
// cannot import a Go peer as plugin.<name> — the pool is not attached to the
// MCP environment, so the peer's exported surface is unreachable in-process.
// A user tool reaches a plugin only through the gated loopback. Skips when
// the demo-go peer isn't built (make in examples/plugins/demo-go).
func TestGoPeerNotInUserTool(t *testing.T) {
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{MCPToolTimeout: 30})
	model.SetRoleCache(nil)

	examples := filepath.Join("..", "..", "examples", "plugins")
	if _, err := os.Stat(examples); err != nil {
		t.Skip("examples/plugins not present")
	}
	registry, err := plugins.Load(examples)
	if err != nil {
		t.Fatal(err)
	}
	plugins.SetRegistry(registry)
	t.Cleanup(func() {
		registry.Close()
		plugins.SetRegistry(nil)
	})
	if registry.ByName("demo-go") == nil {
		t.Skip("demo-go peer not built (make in examples/plugins/demo-go)")
	}

	user := &model.User{Username: "plain", Id: "u-p"}
	script := &model.Script{Name: "go_peer_tool", ScriptType: "tool", Active: true, Content: `
import plugin.demolib as demolib

print(demolib.greeting("knot"))
`}
	if _, err := ExecuteScriptWithMCP(script, map[string]object.Object{}, user); err == nil {
		t.Fatal("user tool imported plugin.demolib, want the import to fail (pool not attached to the MCP env)")
	}
}

// TestUserLibPluginPermissions pins the knot.user library's plugin-grant
// surface against a stubbed loopback: list_plugin_permissions returns the
// resolved grants, and has_permission dispatches on its argument — an
// integer checks the built-in endpoint, a "plugin." string checks the
// grants — the same rule the user global's has_permission follows.
func TestUserLibPluginPermissions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/kai/permissions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"permissions": [2], "plugin_permissions": ["plugin.granter.read"]}`))
	})
	mux.HandleFunc("GET /api/users/kai/has-permission", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"has_permission": ` + map[bool]string{true: "true", false: "false"}[r.URL.Query().Get("permission") == "2"] + `}`))
	})
	rest.SetAPIMux(mux)
	config.SetServerConfig(&config.ServerConfig{MCPToolTimeout: 30})
	model.SetRoleCache(nil)

	user := &model.User{Username: "kai", Id: "u-kai", Active: true}
	script := &model.Script{Name: "perm_lib", ScriptType: "tool", Active: true, Content: `
import knot.user as user

grants = user.list_plugin_permissions("kai")
print("listed:" + str("plugin.granter.read" in grants))
print("builtin:" + str(user.has_permission("kai", 2)))
print("builtin_no:" + str(user.has_permission("kai", 7)))
print("grant:" + str(user.has_permission("kai", "plugin.granter.read")))
print("grant_no:" + str(user.has_permission("kai", "plugin.other.none")))
`}
	out, err := ExecuteScriptWithMCP(script, map[string]object.Object{}, user)
	if err != nil {
		t.Fatalf("perm lib script: %v", err)
	}
	for _, want := range []string{"listed:True", "builtin:True", "builtin_no:False", "grant:True", "grant_no:False"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
}

// TestPluginEnvLibrariesInSync guards the manually-synced allowlist in
// internal/plugins (pluginEnvLibraries): every name metadata dependency
// resolution accepts must import in a real plugin handler environment.
// The reverse direction cannot be enumerated — a registered-but-unlisted
// library only means a dependency declaration on it fails to resolve.
func TestPluginEnvLibrariesInSync(t *testing.T) {
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})
	model.SetRoleCache(nil)

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "guard")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := "# /// script\n# requires-scriptling = \">=0.24\"\n#\n# [tool.knot]\n# version = \"1.0\"\n# ///\ndef unused():\n    return {}\n"
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

	user := &model.User{Username: "guard", Id: "u-g"}
	env, err := NewPluginScriptlingEnv(apiclient.NewMuxClient(user), user, registry.ByName("guard"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range plugins.PluginEnvLibraries() {
		if _, err := env.EvalWithContext(context.Background(), "import "+name); err != nil {
			t.Errorf("pluginEnvLibraries lists %q, but importing it in the plugin env fails: %v", name, err)
		}
	}
}

// TestPluginEnvRequestsFetch pins the plugin env's outbound HTTP end to
// end: not just that requests imports (the drift test above), but that a
// fetch made from inside a plugin handler environment reaches a real HTTP
// server and parses its JSON — the shape of a plugin whose handlers read
// a remote API.
func TestPluginEnvRequestsFetch(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`["main", "feature-x"]`))
	}))
	defer srv.Close()

	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})
	model.SetRoleCache(nil)

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "fetcher")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := "# /// script\n# requires-scriptling = \">=0.24\"\n#\n# [tool.knot]\n# version = \"1.0\"\n# ///\ndef unused():\n    return {}\n"
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

	user := &model.User{Username: "fetcher", Id: "u-f"}
	env, err := NewPluginScriptlingEnv(apiclient.NewMuxClient(user), user, registry.ByName("fetcher"))
	if err != nil {
		t.Fatal(err)
	}
	script := "import requests\n" +
		"r = requests.get('" + srv.URL + "/api/branches/demo', timeout=5)\n" +
		"r.raise_for_status()\n" +
		"branches = r.json()\n" +
		"assert len(branches) == 2, 'wanted 2 branches'\n"
	if _, err := env.EvalWithContext(context.Background(), script); err != nil {
		t.Fatalf("requests fetch in the plugin env failed: %v", err)
	}
	if gotPath != "/api/branches/demo" {
		t.Errorf("test server saw %q, want /api/branches/demo", gotPath)
	}
}

// TestPluginEnvFilesystemUnjailed pins the plugin env's filesystem reach:
// the path-taking libraries are registered unrestricted, so a handler reads
// anywhere the knot process user can — the same authority a binary peer
// has. Subprocess was never jailed, so the old plugin-folder restriction
// was an inconsistency, not a boundary; this test keeps the intended
// behaviour pinned.
func TestPluginEnvFilesystemUnjailed(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "outside-marker.txt")
	if err := os.WriteFile(outside, []byte("beyond the plugin folder"), 0o644); err != nil {
		t.Fatal(err)
	}

	pluginDir := filepath.Join(dir, "fsplugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := "# /// script\n# requires-scriptling = \">=0.24\"\n#\n# [tool.knot]\n# version = \"1.0\"\n# ///\ndef unused():\n    return {}\n"
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

	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})
	model.SetRoleCache(nil)
	user := &model.User{Username: "fsprobe", Id: "u-fs"}
	env, err := NewPluginScriptlingEnv(apiclient.NewMuxClient(user), user, registry.ByName("fsplugin"))
	if err != nil {
		t.Fatal(err)
	}
	script := "import fs\n" +
		"content = fs.read_bytes('" + outside + "', 0, 64)\n" +
		"assert content == 'beyond the plugin folder', content\n"
	if _, err := env.EvalWithContext(context.Background(), script); err != nil {
		t.Fatalf("reading outside the plugin folder failed: %v", err)
	}
}

// TestScriptlingBinPeer pins the scriptling-authored binary peer end to
// end: knot spawns bin/kvstore like any peer, the shebang hands it to the
// scriptling CLI (database drivers compiled in, knot links none of it),
// and handlers reach its functions through the auto-generated stubs —
// with the sqlite file persisting beside the executable. Skips when the
// scriptling CLI is not on PATH (the peer cannot start without it).
func TestScriptlingBinPeer(t *testing.T) {
	if _, err := exec.LookPath("scriptling"); err != nil {
		t.Skip("scriptling CLI not on PATH")
	}
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{MCPToolTimeout: 30})
	model.SetRoleCache(nil)

	examples := filepath.Join("..", "..", "examples", "plugins")
	if _, err := os.Stat(examples); err != nil {
		t.Skip("examples/plugins not present")
	}
	registry, err := plugins.Load(examples)
	if err != nil {
		t.Fatal(err)
	}
	plugins.SetRegistry(registry)
	t.Cleanup(func() {
		registry.Close()
		plugins.SetRegistry(nil)
	})
	demo := registry.ByName("demo-scriptlingcli")
	if demo == nil {
		t.Fatalf("demo-scriptlingcli not loaded; failed = %+v", registry.Failed())
	}

	admin := &model.User{Username: "admin", Id: "u-a", Roles: []string{model.RoleAdminUUID}}
	for i := 0; i < 2; i++ {
		got, err := DispatchPluginHandler(context.Background(), apiclient.NewMuxClient(admin), admin, demo, "col_store", nil)
		if err != nil {
			t.Fatalf("col_store (dispatch %d): %v", i+1, err)
		}
		enc, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"plugin.kvstore (scriptling CLI, sqlite)", "5"} {
			if !strings.Contains(string(enc), want) {
				t.Errorf("dispatch %d missing %q: %s", i+1, want, enc)
			}
		}
	}
}
