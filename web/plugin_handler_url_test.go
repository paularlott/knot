package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
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
#
# [[tool.knot.handlers]]
# handler = "echo_word"
#
# [[tool.knot.handlers]]
# handler = "col_table"
# permission = "read"
#
# [[tool.knot.handlers]]
# handler = "ghost"
# ///
def report():
    return {"rows": [{"columns": [{"id": "t", "type": "text", "handler": "col_text"}]}]}


def open_layout():
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


def col_public():
    return {"text": "public"}


def col_private():
    return {"text": "private"}


def col_rowprivate():
    return {"text": "row private"}


def popup_notes():
    return {"title": "Notes", "markdown": "notes"}


def col_secret():
    return {"text": "never referenced by any layout"}


def echo_word():
    word = params.get("word", "")
    return {"reply": "echo: " + word.upper()}


def col_text():
    return {"text": "plain"}


def col_table():
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
