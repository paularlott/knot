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

// callFixture loads two plugins: a provider exposing a declared handler
// (gated and ungated variants) and a caller that reaches them through
// knot.plugin.call.
func callFixture(t *testing.T) (*plugins.Plugin, *plugins.Plugin) {
	t.Helper()
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})
	model.SetRoleCache(nil)

	dir := t.TempDir()
	for name, source := range map[string]string{
		"provider": `# /// script
# requires-scriptling = ">=0.24"
#
# [tool.knot]
# version = "1.0"
# permissions = ["read"]
#
# [[tool.knot.handlers]]
# handler = "hello"
#
# [[tool.knot.handlers]]
# handler = "whoami"
#
# [[tool.knot.handlers]]
# handler = "how_called"
#
# [[tool.knot.handlers]]
# handler = "secret"
# permission = "read"
# ///
def hello():
    return {"reply": "hello " + params.get("word", "")}


def whoami():
    return {"as_user": user.name, "grant": user.has_permission("plugin.provider.read")}


def how_called():
    return {"method": request.method, "path": request.path}


def secret():
    return {"reply": "the goods"}
`,
		"caller": `# /// script
# requires-scriptling = ">=0.24"
#
# [tool.knot]
# version = "1.0"
# ///
def call_provider():
    import knot.plugin as kp

    return kp.call("provider", params.get("handler", ""), {"word": params.get("word", "")}, method=params.get("method", "GET"))
`,
	} {
		pluginDir := filepath.Join(dir, name)
		if err := os.MkdirAll(pluginDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
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
	return registry.ByName("caller"), registry.ByName("provider")
}

// TestKnotPluginCall pins the in-process bridge: a plugin handler calls
// another plugin's declared handler as the requesting user, undeclared
// handlers are not addressable, and declared gates refuse ungranted users.
func TestKnotPluginCall(t *testing.T) {
	caller, _ := callFixture(t)
	admin := &model.User{Username: "admin", Id: "u-1", Roles: []string{model.RoleAdminUUID}}
	plain := &model.User{Username: "plain", Id: "u-2"}
	ctx := context.Background()

	// Declared, ungated: callable by anyone, params flow through.
	got, err := DispatchPluginHandler(ctx, apiclient.NewMuxClient(plain), plain, caller, "call_provider", map[string]any{"handler": "hello", "word": "world"})
	if err != nil {
		t.Fatalf("call hello: %v", err)
	}
	if reply := got.(map[string]any)["reply"]; reply != "hello world" {
		t.Fatalf("reply = %v, want 'hello world'", reply)
	}

	// Declared, gated: refused for a user without the grant.
	_, err = DispatchPluginHandler(ctx, apiclient.NewMuxClient(plain), plain, caller, "call_provider", map[string]any{"handler": "secret", "word": ""})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("call secret as plain: err = %v, want permission denied", err)
	}

	// Admin passes the gate.
	if _, err := DispatchPluginHandler(ctx, apiclient.NewMuxClient(admin), admin, caller, "call_provider", map[string]any{"handler": "secret", "word": ""}); err != nil {
		t.Fatalf("call secret as admin: %v", err)
	}

	// Undeclared handlers are not addressable at all.
	_, err = DispatchPluginHandler(ctx, apiclient.NewMuxClient(admin), admin, caller, "call_provider", map[string]any{"handler": "no_such", "word": ""})
	if err == nil || !strings.Contains(err.Error(), "not addressable") {
		t.Fatalf("call undeclared: err = %v, want not addressable", err)
	}
}

// TestKnotPluginCallBindsCallerUser pins that the callee's environment is
// bound to the CALLING user: the provider's user global reports the
// caller's name, and their plugin grants answer its in-code checks.
func TestKnotPluginCallBindsCallerUser(t *testing.T) {
	caller, _ := callFixture(t)
	model.SetRoleCache([]*model.Role{{
		Id:                "role-granted",
		Name:              "Granted",
		PluginPermissions: []string{"plugin.provider.read"},
	}})
	granted := &model.User{Username: "kai", Id: "u-3", Roles: []string{"role-granted"}}
	plain := &model.User{Username: "plain", Id: "u-2"}
	ctx := context.Background()

	for _, tc := range []struct {
		user *model.User
		want string
	}{
		{granted, `{"as_user":"kai","grant":true}`},
		{plain, `{"as_user":"plain","grant":false}`},
	} {
		got, err := DispatchPluginHandler(ctx, apiclient.NewMuxClient(tc.user), tc.user, caller, "call_provider", map[string]any{"handler": "whoami", "word": ""})
		if err != nil {
			t.Fatalf("call whoami as %s: %v", tc.user.Username, err)
		}
		// The handler's dict comes back as a Go map; Marshal's sorted keys
		// make the assertion read like the script wrote it.
		enc, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if string(enc) != tc.want {
			t.Errorf("whoami as %s = %s, want %s", tc.user.Username, enc, tc.want)
		}
	}
}


// TestUserHasPermissionDispatch pins the merged permission check: one
// has_permission method whose argument picks the check — an integer is a
// built-in permission id (the knot.permission constants), a "plugin."-
// prefixed string a qualified grant, any other string a built-in key —
// and admins pass all of them without holding any.
func TestUserHasPermissionDispatch(t *testing.T) {
	user := &model.User{
		Username: "tester", Id: "u-10", Groups: []string{"platform"},
		Roles: []string{"role-viewer"},
	}
	model.SetRoleCache([]*model.Role{{
		Id:                "role-viewer",
		Name:              "Viewer",
		Permissions:       []uint16{model.PermissionUseMCPServer},
		PluginPermissions: []string{"plugin.metrics.read"},
	}})
	config.SetServerConfig(&config.ServerConfig{MCPToolTimeout: 30})
	rest.SetAPIMux(http.NewServeMux())
	script := &model.Script{Name: "perm", ScriptType: "tool", Active: true, Content: fmt.Sprintf(`
import json
print(json.dumps({
  "builtin_held": user.has_permission("use_mcp_server"),
  "builtin_lacked": user.has_permission("manage_spaces"),
  "builtin_id_held": user.has_permission(%d),
  "builtin_id_lacked": user.has_permission(%d),
  "builtin_id_unknown": user.has_permission(9999),
  "grant_held": user.has_permission("plugin.metrics.read"),
  "grant_lacked": user.has_permission("plugin.metrics.write"),
  "prefix_alone": user.has_permission("plugin.metrics"),
  "list_arg": user.has_permission(["manage_spaces"]),
  "float_arg": user.has_permission(1.5),
  "bool_arg": user.has_permission(True),
}))
`, model.PermissionUseMCPServer, model.PermissionManageSpaces)}
	out, err := ExecuteScriptWithMCP(script, map[string]object.Object{}, user)
	if err != nil {
		t.Fatalf("ExecuteScriptWithMCP: %v", err)
	}
	for _, want := range []string{
		`"builtin_held":true`,
		`"builtin_lacked":false`,
		`"builtin_id_held":true`,
		`"builtin_id_lacked":false`,
		`"builtin_id_unknown":false`,
		`"grant_held":true`,
		`"grant_lacked":false`,
		`"prefix_alone":false`,
		`"list_arg":false`,
		`"float_arg":false`,
		`"bool_arg":false`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}

	admin := &model.User{Username: "root", Id: "u-11", Roles: []string{model.RoleAdminUUID}}
	out, err = ExecuteScriptWithMCP(script, map[string]object.Object{}, admin)
	if err != nil {
		t.Fatalf("ExecuteScriptWithMCP (admin): %v", err)
	}
	for _, want := range []string{
		`"builtin_lacked":true`,
		`"builtin_id_lacked":true`,
		`"builtin_id_unknown":true`,
		`"grant_lacked":true`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("admin output missing %q: %s", want, out)
		}
	}
}

// TestScriptToolUserSurface pins the identity surface for script tools:
// a script executed with ExecuteScriptWithMCP sees the user it runs as —
// name, groups, permissions, plugin grants — via the `user` dict.
func TestScriptToolUserSurface(t *testing.T) {
	user := &model.User{
		Username: "tester", Id: "u-9", Groups: []string{"platform"},
	}
	config.SetServerConfig(&config.ServerConfig{MCPToolTimeout: 30})
	rest.SetAPIMux(http.NewServeMux())
	script := &model.Script{Name: "whoami", ScriptType: "tool", Active: true, Content: `
import json
print(json.dumps({"name": user.name, "groups": user.groups, "is_admin": user.is_admin, "platform": user.in_group("platform")}))
`}
	model.SetRoleCache(nil)
	out, err := ExecuteScriptWithMCP(script, map[string]object.Object{}, user)
	if err != nil {
		t.Fatalf("ExecuteScriptWithMCP: %v", err)
	}
	for _, want := range []string{"tester", "platform", "false"} {
		if !strings.Contains(out, want) {
			t.Errorf("tool output missing %q: %s", want, out)
		}
	}
}

// TestKnotPluginCallMethodParity pins the in-process bridge's half of the
// converged call contract: the method kwarg (GET default, POST honored)
// reaches the callee's request.method, the synthetic plugin-root path is
// set, and the name validation matches the loopback transport's — so
// call() behaves the same whichever environment it runs in.
func TestKnotPluginCallMethodParity(t *testing.T) {
	caller, _ := callFixture(t)
	admin := &model.User{Username: "admin", Id: "u-1", Roles: []string{model.RoleAdminUUID}}
	ctx := context.Background()

	call := func(extra map[string]any) map[string]any {
		t.Helper()
		got, err := DispatchPluginHandler(ctx, apiclient.NewMuxClient(admin), admin, caller, "call_provider", extra)
		if err != nil {
			t.Fatalf("call_provider: %v", err)
		}
		return got.(map[string]any)
	}

	// Default GET with the synthetic plugin-root path.
	if got := call(map[string]any{"handler": "how_called"}); got["method"] != "GET" || got["path"] != "/plugins/provider/how_called" {
		t.Errorf("default call = %v, want GET at /plugins/provider/how_called", got)
	}
	// The method kwarg reaches the callee's request.method.
	if got := call(map[string]any{"handler": "how_called", "method": "POST"}); got["method"] != "POST" {
		t.Errorf("method=POST call = %v, want POST", got)
	}
	// Name validation matches the loopback: no path separators through.
	if _, err := DispatchPluginHandler(ctx, apiclient.NewMuxClient(admin), admin, caller, "call_provider", map[string]any{"handler": "report/col_text"}); err == nil || !strings.Contains(err.Error(), "invalid plugin or handler name") {
		t.Errorf("path in handler name: err = %v, want invalid plugin or handler name", err)
	}
}
