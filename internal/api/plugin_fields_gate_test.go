package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
)

// fieldGateFixture loads a plugin with two field handlers: one gated by a
// declared permission, one open to anyone who passes the route's UseSpaces
// middleware.
func fieldGateFixture(t *testing.T) {
	t.Helper()
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "fields")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `# /// script
# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0"
# permissions = ["pick"]
#
# [[tool.knot.field_handlers]]
# label = "Environments"
# handler = "field_env"
# permission = "pick"
##
# [[tool.knot.field_handlers]]
# label = "Open"
# handler = "field_open"
# ///
def field_env(request):
    return {"options": [{"key": "dev", "text": "dev"}]}


def field_open(request):
    return {"options": [{"key": "any", "text": "any"}]}
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

func callFieldHandler(t *testing.T, handlerId string, user *model.User) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/plugins/field-handlers/"+handlerId, nil)
	r = r.WithContext(context.WithValue(r.Context(), "user", user))
	r.SetPathValue("handler_id", handlerId)
	w := httptest.NewRecorder()
	HandleGetPluginFieldHandlerOptions(w, r)
	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

// TestFieldHandlerOptionGate pins the additive gate: UseSpaces is the
// baseline (middleware, not tested here), and a field handler's declared
// permission narrows who may invoke it.
func TestFieldHandlerOptionGate(t *testing.T) {
	model.SetRoleCache(nil)
	fieldGateFixture(t)
	admin := &model.User{Username: "admin", Roles: []string{model.RoleAdminUUID}}
	plain := &model.User{Username: "plain"}

	// Gated handler: plain users are refused, admins pass.
	w, _ := callFieldHandler(t, "plugin.fields.field_env", plain)
	if w.Code != http.StatusForbidden {
		t.Fatalf("gated handler for plain user: status = %d, want 403", w.Code)
	}
	w, body := callFieldHandler(t, "plugin.fields.field_env", admin)
	if w.Code != http.StatusOK {
		t.Fatalf("gated handler for admin: status = %d, body = %s", w.Code, w.Body.String())
	}
	if options, ok := body["options"].([]any); !ok || len(options) != 1 {
		t.Fatalf("gated handler options = %v", body)
	}

	// Ungated handler: any authenticated user passes.
	w, body = callFieldHandler(t, "plugin.fields.field_open", plain)
	if w.Code != http.StatusOK {
		t.Fatalf("ungated handler for plain user: status = %d, body = %s", w.Code, w.Body.String())
	}
	if options, ok := body["options"].([]any); !ok || len(options) != 1 {
		t.Fatalf("ungated handler options = %v", body)
	}

	// Unknown handler id is a 404.
	if w, _ := callFieldHandler(t, "plugin.fields.no_such", admin); w.Code != http.StatusNotFound {
		t.Fatalf("unknown handler: status = %d, want 404", w.Code)
	}
}
