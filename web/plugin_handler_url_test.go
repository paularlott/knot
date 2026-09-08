package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling/object"
)

// handlerURLFixture loads a one-plugin registry exercising every gate shape:
// a permission-gated /report page, handlers with and without their own
// [[tool.knot.handlers]] declarations, and an undeclared page handler.
func handlerURLFixture(t *testing.T) {
	t.Helper()
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "hooked")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `# /// script
# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0"
# permissions = ["read"]
#
# [[tool.knot.pages]]
# path = "/report"
# handler = "report"
# label = "Report"
# permission = "read"
#
# [[tool.knot.pages]]
# path = "/open"
# handler = "open_layout"
# label = "Open"
##
# [[tool.knot.handlers]]
# handler = "echo_word"
#
# [[tool.knot.handlers]]
# handler = "col_table"
# permission = "read"
##
# [[tool.knot.handlers]]
# handler = "ghost"
# ///
def report(request):
    return {"rows": [{"columns": [{"id": "t", "type": "text", "handler": "col_text"}]}]}


def open_layout(request):
    # A public page whose columns carry their own gates: the gated column's
    # handler must refuse users who fail the column permission, and a row
    # gate must hide its columns' handlers entirely.
    return {"rows": [
        {"columns": [
            {"id": "pub", "type": "text", "handler": "col_public"},
            {"id": "priv", "type": "text", "handler": "col_private", "permission": "read",
             "actions": [{"action": "note", "label": "Note", "handler": "popup_notes"}]},
        ]},
        {"permission": "read", "columns": [
            {"id": "rowpriv", "type": "text", "handler": "col_rowprivate"},
        ]},
    ]}


def col_public(request):
    return {"text": "public"}


def col_private(request):
    return {"text": "private"}


def col_rowprivate(request):
    return {"text": "row private"}


def popup_notes(request):
    return {"title": "Notes", "markdown": "notes"}


def col_secret(request):
    return {"text": "never referenced by any layout"}


def echo_word(request):
    params = request["params"]
    word = params.get("word", "")
    return {"reply": "echo: " + word.upper()}


def col_text(request):
    return {"text": "plain"}


def col_table(request):
    return {"columns": [{"key": "name", "label": "Name"}], "rows": [{"name": "alpha"}]}
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
}

// dispatchPluginRequest routes a request through the plugin page routes with
// the given user in context, mirroring middleware.WebAuth.
func dispatchPluginRequest(t *testing.T, method, target string, user *model.User) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /plugins/{plugin_name}/{path...}", HandlePluginPage)
	mux.HandleFunc("POST /plugins/{plugin_name}/{path...}", HandlePluginPage)
	r := httptest.NewRequest(method, target, nil)
	r = r.WithContext(context.WithValue(r.Context(), "user", user))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// TestPluginHandlerURLDispatch pins the handler-URL surface: a declared
// handler rides a declared page's path (/plugins/x/page/handler) or the
// plugin root (/plugins/x/handler), and always answers JSON.
func TestPluginHandlerURLDispatch(t *testing.T) {
	model.SetRoleCache(nil)
	handlerURLFixture(t)
	admin := &model.User{Username: "admin", Roles: []string{model.RoleAdminUUID}}

	// Page-scoped handler URL.
	w := dispatchPluginRequest(t, "GET", "/plugins/hooked/report/echo_word?word=hi", admin)
	if w.Code != http.StatusOK {
		t.Fatalf("page-scoped handler status = %d, body = %s", w.Code, w.Body.String())
	}
	var reply struct {
		Reply string `json:"reply"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || reply.Reply != "echo: HI" {
		t.Fatalf("page-scoped handler body = %s (%v)", w.Body.String(), err)
	}

	// Plugin-root handler URL: same declared handler without a page.
	w = dispatchPluginRequest(t, "GET", "/plugins/hooked/echo_word", admin)
	if w.Code != http.StatusOK {
		t.Fatalf("plugin-root handler status = %d, body = %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || reply.Reply != "echo: " {
		t.Fatalf("plugin-root handler body = %s (%v)", w.Body.String(), err)
	}

	// A handler's table payload passes through untouched (its "rows" are
	// data, not a page layout to re-normalize).
	w = dispatchPluginRequest(t, "GET", "/plugins/hooked/col_table", admin)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "alpha") {
		t.Fatalf("table handler = %d %s", w.Code, w.Body.String())
	}

	// Declared but nonexistent function: JSON error, not a page render.
	w = dispatchPluginRequest(t, "GET", "/plugins/hooked/ghost", admin)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("missing function status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "handler failed") {
		t.Fatalf("missing function body = %s", w.Body.String())
	}

	// Undeclared handlers are not root-addressable; unknown plugins and
	// multi-segment leftovers are 404s.
	if w := dispatchPluginRequest(t, "GET", "/plugins/hooked/col_text", admin); w.Code != http.StatusNotFound {
		t.Fatalf("undeclared root handler status = %d, want 404", w.Code)
	}
	if w := dispatchPluginRequest(t, "GET", "/plugins/nope/echo_word", admin); w.Code != http.StatusNotFound {
		t.Fatalf("unknown plugin status = %d", w.Code)
	}
	if w := dispatchPluginRequest(t, "GET", "/plugins/hooked/report/a/b", admin); w.Code != http.StatusNotFound {
		t.Fatalf("multi-segment status = %d", w.Code)
	}
}

// TestPluginColumnGateAtFetch pins that column gates are enforced when the
// handler is fetched, not just when the layout is pruned: an undeclared
// handler is only reachable through a page if the requesting user's
// normalized layout offers it (column permission, row permission), and
// handlers no layout references are refused even for admins. Declared
// handlers stand on their own gate and skip the layout check.
func TestPluginColumnGateAtFetch(t *testing.T) {
	model.SetRoleCache(nil)
	handlerURLFixture(t)
	admin := &model.User{Username: "admin", Roles: []string{model.RoleAdminUUID}}
	plain := &model.User{Username: "plain"}

	cases := []struct {
		target string
		user   *model.User
		want   int
		why    string
	}{
		// Public page, ungated column: everyone who can see the page.
		{"/plugins/hooked/open/col_public", plain, http.StatusOK, "ungated column handler serves plain users"},
		// Public page, column-gated handler: the column gate holds at fetch.
		{"/plugins/hooked/open/col_private", plain, http.StatusForbidden, "column permission holds at fetch time"},
		{"/plugins/hooked/open/col_private", admin, http.StatusOK, "admin passes the column permission"},
		// Row-gated column: the row gate hides the handler too.
		{"/plugins/hooked/open/col_rowprivate", plain, http.StatusForbidden, "row permission hides the column handler"},
		{"/plugins/hooked/open/col_rowprivate", admin, http.StatusOK, "admin passes the row permission"},
		// An action's popup handler rides its column's visibility.
		{"/plugins/hooked/open/popup_notes", plain, http.StatusForbidden, "action handler follows its column's gate"},
		{"/plugins/hooked/open/popup_notes", admin, http.StatusOK, "admin reaches the action handler"},
		// No layout references this handler: not callable, even by admins.
		{"/plugins/hooked/open/col_secret", admin, http.StatusForbidden, "unreferenced handler is not callable"},
		{"/plugins/hooked/report/col_secret", admin, http.StatusForbidden, "unreferenced handler is not callable on any page"},
		// Declared handlers skip the layout check and use their own gate.
		{"/plugins/hooked/report/echo_word", admin, http.StatusOK, "declared handler stands on its declaration"},
	}
	for _, tc := range cases {
		w := dispatchPluginRequest(t, "GET", tc.target, tc.user)
		if w.Code != tc.want {
			t.Errorf("%s as %s: status = %d, want %d (%s)", tc.target, tc.user.Username, w.Code, tc.want, tc.why)
		}
	}
}

// TestLayoutPresenceCache pins the memoization: a page's column burst runs
// the layout handler once (per user and query), refusals come from the same
// memoized answer, POSTs never use the cache, and entries expire.
func TestLayoutPresenceCache(t *testing.T) {
	model.SetRoleCache(nil)
	handlerURLFixture(t)
	layoutPresence.resetForTest(time.Minute)
	admin := &model.User{Username: "admin", Roles: []string{model.RoleAdminUUID}}
	plain := &model.User{Username: "plain"}

	// First fetch evaluates and memoizes the layout.
	if w := dispatchPluginRequest(t, "GET", "/plugins/hooked/open/col_public", admin); w.Code != http.StatusOK {
		t.Fatalf("first fetch: status = %d", w.Code)
	}
	if hits, misses := layoutPresence.stats(); hits != 0 || misses != 1 {
		t.Fatalf("after first fetch: hits = %d, misses = %d, want 0/1", hits, misses)
	}

	// The burst reuses the entry: no further layout evaluations.
	for i := 0; i < 3; i++ {
		if w := dispatchPluginRequest(t, "GET", "/plugins/hooked/open/col_public", admin); w.Code != http.StatusOK {
			t.Fatalf("burst fetch %d: status = %d", i, w.Code)
		}
	}
	if hits, misses := layoutPresence.stats(); hits != 3 || misses != 1 {
		t.Fatalf("after burst: hits = %d, misses = %d, want 3/1", hits, misses)
	}

	// Refusals read the same entry: an unoffered handler 403s without
	// re-running the layout.
	if w := dispatchPluginRequest(t, "GET", "/plugins/hooked/open/col_secret", admin); w.Code != http.StatusForbidden {
		t.Fatalf("unoffered handler: status = %d, want 403", w.Code)
	}
	if hits, _ := layoutPresence.stats(); hits != 4 {
		t.Fatalf("refusal should hit the cache: hits = %d, want 4", hits)
	}

	// POST never reads the cache.
	if w := dispatchPluginRequest(t, "POST", "/plugins/hooked/open/col_public", admin); w.Code != http.StatusOK {
		t.Fatalf("POST fetch: status = %d", w.Code)
	}
	if hits, misses := layoutPresence.stats(); hits != 4 || misses != 1 {
		t.Fatalf("after POST: hits = %d, misses = %d, want 4/1 (POST bypasses the cache)", hits, misses)
	}

	// A different query is a different answer: the layout may vary with
	// params, so it re-evaluates.
	if w := dispatchPluginRequest(t, "GET", "/plugins/hooked/open/col_public?zone=a", admin); w.Code != http.StatusOK {
		t.Fatalf("query variant: status = %d", w.Code)
	}
	if _, misses := layoutPresence.stats(); misses != 2 {
		t.Fatalf("query variant should miss: misses = %d, want 2", misses)
	}

	// A different user has their own entry (their gates differ).
	if w := dispatchPluginRequest(t, "GET", "/plugins/hooked/open/col_public", plain); w.Code != http.StatusOK {
		t.Fatalf("other user: status = %d", w.Code)
	}
	if _, misses := layoutPresence.stats(); misses != 3 {
		t.Fatalf("other user should miss: misses = %d, want 3", misses)
	}
}

// TestLayoutPresenceCacheExpiry pins the TTL: an expired entry re-evaluates.
func TestLayoutPresenceCacheExpiry(t *testing.T) {
	model.SetRoleCache(nil)
	handlerURLFixture(t)
	layoutPresence.resetForTest(time.Nanosecond)
	admin := &model.User{Username: "admin", Roles: []string{model.RoleAdminUUID}}

	if w := dispatchPluginRequest(t, "GET", "/plugins/hooked/open/col_public", admin); w.Code != http.StatusOK {
		t.Fatalf("first fetch: status = %d", w.Code)
	}
	time.Sleep(2 * time.Millisecond)
	if w := dispatchPluginRequest(t, "GET", "/plugins/hooked/open/col_public", admin); w.Code != http.StatusOK {
		t.Fatalf("post-expiry fetch: status = %d", w.Code)
	}
	if _, misses := layoutPresence.stats(); misses != 2 {
		t.Fatalf("expired entry should re-evaluate: misses = %d, want 2", misses)
	}

	layoutPresence.resetForTest(layoutPresenceTTL)
}

// TestPluginHandlerURLGate pins the gate matrix: a handler's own
// [[tool.knot.handlers]] declaration is authoritative wherever it is called
// (page path or plugin root); an undeclared handler inherits
// the calling page's gate and no other.
func TestPluginHandlerURLGate(t *testing.T) {
	model.SetRoleCache(nil)
	handlerURLFixture(t)
	admin := &model.User{Username: "admin", Roles: []string{model.RoleAdminUUID}}
	plain := &model.User{Username: "plain"}

	cases := []struct {
		target string
		user   *model.User
		want   int
		why    string
	}{
		// echo_word declares no gate: any logged-in user, everywhere.
		{"/plugins/hooked/echo_word", plain, http.StatusOK, "declared without gate: root, any user"},
		{"/plugins/hooked/report/echo_word", plain, http.StatusOK, "declared without gate overrides the page's gate"},
		// col_table declares permission read: gated everywhere.
		{"/plugins/hooked/col_table", plain, http.StatusForbidden, "declared permission applies at the root"},
		{"/plugins/hooked/report/col_table", plain, http.StatusForbidden, "declared permission applies on the page path"},
		{"/plugins/hooked/col_table", admin, http.StatusOK, "admin passes the declared permission"},
		// col_text has no declaration: the page's gate governs, root is 404.
		{"/plugins/hooked/report/col_text", plain, http.StatusForbidden, "undeclared handler inherits the page gate"},
		{"/plugins/hooked/report/col_text", admin, http.StatusOK, "admin passes the page gate"},
		{"/plugins/hooked/col_text", plain, http.StatusNotFound, "undeclared handler is not root-addressable"},
	}
	for _, tc := range cases {
		w := dispatchPluginRequest(t, "GET", tc.target, tc.user)
		if w.Code != tc.want {
			t.Errorf("%s as %s: status = %d, want %d (%s)", tc.target, tc.user.Username, w.Code, tc.want, tc.why)
		}
	}
}

// userGlobalFixture loads a one-plugin registry whose column handler echoes
// the user global's identity surface as JSON text.
func userGlobalFixture(t *testing.T) {
	t.Helper()
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "whoami")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`# /// script
# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0"
# permissions = ["read"]
#
# [[tool.knot.pages]]
# path = "/me"
# handler = "me_layout"
# label = "Me"
# ///
def me_layout(request):
    return {"rows": [{"columns": [{"id": "me", "type": "text", "handler": "col_me"}]}]}


def col_me(request):
    import json
    import knot.identity

    # request["user"] is inert data (name, is_admin, groups); the
    # authoritative permission surface is knot.identity.user(), bound to the
    # requesting user per dispatch over the gated loopback.
    me = knot.identity.user()
    return {"text": json.dumps({
        "name": request["user"]["name"],
        "group": me.in_group("platform"),
        "admin": request["user"]["is_admin"],
        "key_held": me.has_permission("use_mcp_server"),
        "key_lacked": me.has_permission("manage_spaces"),
        "id_held": me.has_permission(%d),
        "grant_held": me.has_permission("plugin.whoami.read"),
        "grant_lacked": me.has_permission("plugin.whoami.write"),
        "list_arg": me.has_permission(["manage_spaces"]),
    })}
`, model.PermissionUseMCPServer)
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
}

// TestPluginUserGlobal pins the user binding on the web dispatch path: a
// column handler fetched through the page sees the requesting user's
// identity and grants (key, id and grant forms all answer), and the very
// next request as a different user is rebound — the pooled environment
// never leaks one user's identity into another's dispatch.
func TestPluginUserGlobal(t *testing.T) {
	model.SetRoleCache([]*model.Role{{
		Id:                "role-who",
		Name:              "Who",
		Permissions:       []uint16{model.PermissionUseMCPServer},
		PluginPermissions: []string{"plugin.whoami.read"},
	}})
	userGlobalFixture(t)
	viewer := &model.User{Username: "viewer", Id: "u-v", Groups: []string{"platform"}, Roles: []string{"role-who"}}
	admin := &model.User{Username: "root", Id: "u-a", Roles: []string{model.RoleAdminUUID}}

	// The handler's JSON rides inside the text column as a string; unwrap
	// it so the assertions read the script's own keys.
	fetch := func(user *model.User) string {
		t.Helper()
		w := dispatchPluginRequest(t, "GET", "/plugins/whoami/me/col_me", user)
		if w.Code != http.StatusOK {
			t.Fatalf("%s fetch status = %d, body = %s", user.Username, w.Code, w.Body.String())
		}
		var col struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &col); err != nil {
			t.Fatalf("%s fetch body = %s (%v)", user.Username, w.Body.String(), err)
		}
		return col.Text
	}

	for _, want := range []string{
		`"name":"viewer"`,
		`"group":true`,
		`"admin":false`,
		`"key_held":true`,
		`"key_lacked":false`,
		`"id_held":true`,
		`"grant_held":true`,
		`"grant_lacked":false`,
		`"list_arg":false`,
	} {
		if text := fetch(viewer); !strings.Contains(text, want) {
			t.Errorf("viewer identity missing %q: %s", want, text)
		}
	}

	// Immediately after, as admin: same pooled env, rebound identity.
	for _, want := range []string{
		`"name":"root"`,
		`"group":false`,
		`"admin":true`,
		`"key_lacked":true`,
		`"grant_lacked":true`,
		`"list_arg":true`,
	} {
		if text := fetch(admin); !strings.Contains(text, want) {
			t.Errorf("admin identity missing %q: %s", want, text)
		}
	}
}

// TestUserToolPluginLoopbackCall pins the user-tool boundary transport:
// knot.plugin.call exists in user-created tools but rides the in-process
// loopback — the same authenticated transport the knot.* libraries use —
// through the real web dispatch. Declared gates apply exactly as for a
// browser fetch, and undeclared handlers are not addressable.
func TestUserToolPluginLoopbackCall(t *testing.T) {
	model.SetRoleCache(nil)
	handlerURLFixture(t)
	config.SetServerConfig(&config.ServerConfig{MCPToolTimeout: 30})

	// The loopback: the plugin routes on the mux the MuxClient hits,
	// wrapped in the same middleware a real request passes through.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /plugins/{plugin_name}/{path...}", middleware.WebAuth(HandlePluginPage))
	mux.HandleFunc("POST /plugins/{plugin_name}/{path...}", middleware.WebAuth(HandlePluginPage))
	rest.SetAPIMux(mux)
	t.Cleanup(func() { rest.SetAPIMux(http.NewServeMux()) })

	plain := &model.User{Username: "plain", Id: "u-p", Active: true}
	admin := &model.User{Username: "admin", Id: "u-a", Active: true, Roles: []string{model.RoleAdminUUID}}

	run := func(user *model.User, body string) (string, error) {
		script := &model.Script{Name: "loopback_tool", ScriptType: "tool", Active: true, Content: body}
		return service.ExecuteScriptWithMCP(script, map[string]object.Object{}, user)
	}

	// Declared, ungated handler: params travel, the JSON body parses.
	out, err := run(plain, "import knot.plugin as kp\nresult = kp.call(\"hooked\", \"echo_word\", {\"word\": \"hi\"})\nprint(result.get(\"reply\"))")
	if err != nil {
		t.Fatalf("loopback call as plain: %v", err)
	}
	if !strings.Contains(out, "echo: HI") {
		t.Errorf("loopback result = %q", out)
	}

	// POST rides the same loopback with a JSON body.
	out, err = run(plain, "import knot.plugin as kp\nresult = kp.call(\"hooked\", \"echo_word\", {\"word\": \"yo\"}, method=\"POST\")\nprint(result.get(\"reply\"))")
	if err != nil || !strings.Contains(out, "echo: YO") {
		t.Errorf("loopback POST = %q err = %v", out, err)
	}

	// Declared, gated handler: refused for a user without the grant.
	if _, err := run(plain, "import knot.plugin as kp\nresult = kp.call(\"hooked\", \"col_table\", {})"); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("gated handler as plain: err = %v, want permission denied", err)
	}
	// Admin passes the same gate through the same path.
	if out, err = run(admin, "import knot.plugin as kp\nimport json\nresult = kp.call(\"hooked\", \"col_table\", {})\nprint(json.dumps(result))"); err != nil || !strings.Contains(out, "alpha") {
		t.Errorf("gated handler as admin = %q err = %v", out, err)
	}

	// Undeclared handlers are not addressable at the plugin root.
	if _, err := run(admin, "import knot.plugin as kp\nresult = kp.call(\"hooked\", \"col_text\", {})"); err == nil || !strings.Contains(err.Error(), "not addressable") {
		t.Fatalf("undeclared handler: err = %v, want not addressable", err)
	}
	// Unknown plugin.
	if _, err := run(admin, "import knot.plugin as kp\nresult = kp.call(\"nope\", \"echo_word\", {})"); err == nil || !strings.Contains(err.Error(), "not addressable") {
		t.Fatalf("unknown plugin: err = %v", err)
	}
}

// TestUserToolPluginReachBoundary enumerates every route a user-created
// tool can take to plugin code and pins the gate on each. The helper
// addresses declared handlers only — a handler name with a path separator
// is refused outright, so page-scoped URLs cannot be smuggled through it.
// The raw loopback (knot.apiclient) reaches exactly the URLs a browser
// can: page-scoped handlers the requesting user's layout offers, with
// column gates at fetch time — never an arbitrary function.
func TestUserToolPluginReachBoundary(t *testing.T) {
	model.SetRoleCache(nil)
	handlerURLFixture(t)
	config.SetServerConfig(&config.ServerConfig{MCPToolTimeout: 30})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /plugins/{plugin_name}/{path...}", middleware.WebAuth(HandlePluginPage))
	mux.HandleFunc("POST /plugins/{plugin_name}/{path...}", middleware.WebAuth(HandlePluginPage))
	rest.SetAPIMux(mux)
	t.Cleanup(func() { rest.SetAPIMux(http.NewServeMux()) })

	admin := &model.User{Username: "admin", Id: "u-a", Active: true, Roles: []string{model.RoleAdminUUID}}

	run := func(body string) error {
		script := &model.Script{Name: "boundary_tool", ScriptType: "tool", Active: true, Content: body}
		_, err := service.ExecuteScriptWithMCP(script, map[string]object.Object{}, admin)
		return err
	}

	// The helper: a handler name carrying a path separator is refused
	// before any request — page-scoped URLs cannot be smuggled in.
	for _, handler := range []string{"report/col_text", "../hooked/echo_word", "open/popup_notes"} {
		if err := run("import knot.plugin as kp\nresult = kp.call(\"hooked\", \"" + handler + "\", {})"); err == nil || !strings.Contains(err.Error(), "invalid plugin or handler name") {
			t.Errorf("call(%q): err = %v, want invalid handler name", handler, err)
		}
	}

	// The raw loopback: exactly the browser's surface. An undeclared
	// handler is not root-addressable...
	if err := run("import knot.apiclient as api\nresult = api.get(\"/plugins/hooked/col_text\")"); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("root undeclared via raw loopback: err = %v, want HTTP 404", err)
	}
	// ...a handler no layout offers is refused even for admins...
	if err := run("import knot.apiclient as api\nresult = api.get(\"/plugins/hooked/open/col_secret\")"); err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("unreferenced handler via raw loopback: err = %v, want HTTP 403", err)
	}
	// ...and one the layout offers answers, gates applied — the same
	// fetch the user's browser makes when it renders the page.
	out, err := service.ExecuteScriptWithMCP(&model.Script{Name: "boundary_tool", ScriptType: "tool", Active: true, Content: "import knot.apiclient as api\nresult = api.get(\"/plugins/hooked/report/col_text\")\nprint(result.get(\"text\"))"}, map[string]object.Object{}, admin)
	if err != nil || !strings.Contains(out, "plain") {
		t.Errorf("offered page-scoped handler via raw loopback = %q err = %v, want the column's data (browser-equivalent)", out, err)
	}

	// Nothing from the plugin's folder is importable either: not the
	// entry file, not its internal modules — only the plugin's published
	// libs/ ever register under the plugin.* namespace.
	for _, name := range []string{"main", "helpers"} {
		if err := run("import " + name); err == nil || !strings.Contains(err.Error(), "unknown library") {
			t.Errorf("import %s in a user tool: err = %v, want unknown library", name, err)
		}
	}
}

// moduleFixture loads a plugin whose handlers live in a sibling module:
// the entry file stays the declaration surface, the bulk of the code
// splits into helpers.py — referenced as "module.fn" everywhere a handler
// name is accepted.
func moduleFixture(t *testing.T) {
	t.Helper()
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "splitter")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := `# /// script
# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0"
#
# [[tool.knot.pages]]
# path = "/report"
# handler = "report"
# label = "Report"
#
# [[tool.knot.handlers]]
# handler = "helpers.echo"
# ///
import helpers


def report(request):
    return {"rows": [{"columns": [{"id": "t", "type": "text", "handler": "helpers.col_data"}]}]}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	module := `def col_data(request):
    return {"text": "data from the module"}


def echo(request):
    return {"reply": "module echo"}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "helpers.py"), []byte(module), 0o644); err != nil {
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
}

// TestModuleHandlerDispatch pins the split: a plugin's handlers may live
// in sibling modules, referenced as "module.fn" in page layouts and
// handler declarations alike — the entry file stays the declaration
// surface while the code scales into as many files as it needs.
func TestModuleHandlerDispatch(t *testing.T) {
	model.SetRoleCache(nil)
	moduleFixture(t)
	admin := &model.User{Username: "admin", Roles: []string{model.RoleAdminUUID}}

	// The layout itself renders with a module-qualified column handler.
	w := dispatchPluginRequest(t, "GET", "/plugins/splitter/report", admin)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "helpers.col_data") {
		t.Fatalf("layout with module handler = %d %s", w.Code, w.Body.String())
	}
	// The column handler answers through its page path (undeclared: the
	// layout offers it, column gates at fetch time).
	w = dispatchPluginRequest(t, "GET", "/plugins/splitter/report/helpers.col_data", admin)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "data from the module") {
		t.Fatalf("module column handler = %d %s", w.Code, w.Body.String())
	}
	// A module handler declared in [[tool.knot.handlers]] is plugin-root
	// addressable like any declared handler.
	w = dispatchPluginRequest(t, "GET", "/plugins/splitter/helpers.echo", admin)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "module echo") {
		t.Fatalf("declared module handler = %d %s", w.Code, w.Body.String())
	}
}
