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

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
)

// handlerURLFixture loads a one-plugin registry whose /report page (the
// default) is permission-gated, with handlers exercising every dispatch shape.
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
# default = true
# ///
def report():
    return {"rows": [{"columns": [{"id": "t", "type": "text", "handler": "col_text"}]}]}


def echo_word():
    word = params.get("word", "")
    return {"reply": "echo: " + word.upper()}


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

// TestPluginHandlerURLDispatch pins the handler-URL surface: a handler rides
// a declared page's path (/plugins/x/page/handler) or the plugin root
// (/plugins/x/handler, gated by the default page), and always answers JSON.
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

	// Plugin-root handler URL: same handler through the default page's gate.
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

	// Unknown function: JSON error, not a page render.
	w = dispatchPluginRequest(t, "GET", "/plugins/hooked/no_such_fn", admin)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("missing handler status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "handler failed") {
		t.Fatalf("missing handler body = %s", w.Body.String())
	}

	// Unknown plugin and multi-segment leftovers are 404s.
	if w := dispatchPluginRequest(t, "GET", "/plugins/nope/echo_word", admin); w.Code != http.StatusNotFound {
		t.Fatalf("unknown plugin status = %d", w.Code)
	}
	if w := dispatchPluginRequest(t, "GET", "/plugins/hooked/report/a/b", admin); w.Code != http.StatusNotFound {
		t.Fatalf("multi-segment status = %d", w.Code)
	}
}

// TestPluginHandlerURLGate pins the gate: the default page's permission
// guards plugin-root handler calls for the requesting user.
func TestPluginHandlerURLGate(t *testing.T) {
	model.SetRoleCache(nil)
	handlerURLFixture(t)
	plain := &model.User{Username: "plain"}

	for _, target := range []string{"/plugins/hooked/echo_word", "/plugins/hooked/report/echo_word"} {
		w := dispatchPluginRequest(t, "GET", target, plain)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s status = %d, want 403 (page gate applies to handler URLs)", target, w.Code)
		}
	}
}
