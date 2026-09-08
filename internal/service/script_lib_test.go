package service

import (
	"context"
	"encoding/json"
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
def add_up():
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

// TestScriptLibInUserTool pins the user-tool surface: plugin-exported
// functions, classes and libs import as plugin.<name> inside user-created
// MCP tools, and the peer's code can read the `user` global to self-gate —
// the plugin's own permission decision, since no metadata gate applies to
// a plain import.
func TestScriptLibInUserTool(t *testing.T) {
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{MCPToolTimeout: 30})
	model.SetRoleCache([]*model.Role{{
		Id:                "role-granted",
		Name:              "Granted",
		PluginPermissions: []string{"plugin.guarded.special"},
	}})
	granted := &model.User{Username: "kai", Id: "u-g", Roles: []string{"role-granted"}}
	plain := &model.User{Username: "plain", Id: "u-p"}

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
# permissions = ["special"]
# ///
def unused():
    return {}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	peer := `import knot.identity


def open_add(a, b):
    return a + b


def granted_only():
    # Module code can't see the user global — its scope is the calling
    # program — so the identity library carries the same instance.
    if not knot.identity.user().has_permission("plugin.guarded.special"):
        raise Exception("plugin.guarded.special not granted")
    return {"ok": True}
`
	if err := os.WriteFile(filepath.Join(libsDir, "calc.py"), []byte(peer), 0o644); err != nil {
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

	run := func(user *model.User, body string) (string, error) {
		script := &model.Script{Name: "peer_tool", ScriptType: "tool", Active: true, Content: body}
		return ExecuteScriptWithMCP(script, map[string]object.Object{}, user)
	}

	// The exported function is callable from a user tool.
	out, err := run(plain, "import plugin.calc as calc\nprint(calc.open_add(2, 3))")
	if err != nil {
		t.Fatalf("user tool calling exported function: %v", err)
	}
	if !strings.Contains(out, "5") {
		t.Errorf("open_add output = %q", out)
	}

	// Self-gating: the peer refuses an ungranted user, passes a granted one.
	// (The result is assigned before use — an exception raised in argument
	// position, print(fn()), is swallowed by scriptling's evaluator.)
	if _, err := run(plain, "import plugin.calc as calc\nresult = calc.granted_only()\nprint(result)"); err == nil || !strings.Contains(err.Error(), "plugin.guarded.special not granted") {
		t.Fatalf("granted_only as plain: err = %v, want the peer's refusal", err)
	}
	if out, err := run(granted, "import plugin.calc as calc\nresult = calc.granted_only()\nprint(result)"); err != nil || !strings.Contains(out, "True") {
		t.Fatalf("granted_only as granted: out = %q err = %v", out, err)
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

// TestGoPeerInUserTool pins the other half of the user-tool peer surface:
// binary (Go) peers import as plugin.<name> through the scriptling plugin
// support — the host-side stubs auto-generated from the peer's handshake,
// the same surface plugin envs get. A user tool calls into the already
// spawned peer process; no control library rides along. Skips when the
// demo-go peer isn't built (make in examples/plugins/demo-go).
func TestGoPeerInUserTool(t *testing.T) {
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

result = demolib.greeting("knot")
info = demolib.status()
print(result + " [" + info.get("peer", "?") + "]")
`}
	out, err := ExecuteScriptWithMCP(script, map[string]object.Object{}, user)
	if err != nil {
		t.Fatalf("user tool calling the Go peer: %v", err)
	}
	if !strings.Contains(out, "hello knot, from a Go peer inside knot") || !strings.Contains(out, "[demolib]") {
		t.Errorf("go peer output = %q", out)
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
