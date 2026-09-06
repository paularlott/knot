package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
)

func pageAdminUser() *model.User {
	return &model.User{Username: "admin", Roles: []string{model.RoleAdminUUID}}
}

func demoPlugin() *plugins.Plugin {
	return &plugins.Plugin{Name: "metrics", Permissions: []plugins.PermissionDecl{{Id: "plugin.metrics.read"}}}
}

// TestNormalizePageDocument covers the layout contract: rows of columns
// with width clamping, gate enforcement (permission and group), empty-row
// pruning, and malformed payloads degrading to error documents.
func TestNormalizePageDocument(t *testing.T) {
	config.SetServerConfig(&config.ServerConfig{})

	doc := normalizePageDocument(map[string]any{
		"rows": []any{
			map[string]any{
				"title": "Fleet",
				"columns": []any{
					map[string]any{"id": "kpi", "type": "stats", "handler": "kpi", "width": 9},
					map[string]any{"type": "chart", "handler": "cpu"},
					map[string]any{"id": "secret", "type": "table", "handler": "admin_table", "permission": "read"},
					map[string]any{"id": "ops", "type": "form", "handler": "ops_form", "group": "platform"},
				},
			},
			map[string]any{"columns": []any{
				map[string]any{"id": "x", "type": "table", "handler": "t", "permission": "read"},
			}},
		},
	}, pageAdminUser(), demoPlugin())

	rows := doc["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (admin passes permission gates)", len(rows))
	}
	row := rows[0].(map[string]any)
	if row["title"] != "Fleet" {
		t.Errorf("title = %v", row["title"])
	}
	columns := row["columns"].([]any)
	if len(columns) != 3 {
		t.Fatalf("columns = %d, want 3 (admin passes permission gates, fails the group gate)", len(columns))
	}
	first := columns[0].(map[string]any)
	if first["width"] != 4 {
		t.Errorf("width clamp = %v, want 4", first["width"])
	}
	if first["id"] != "kpi" {
		t.Errorf("id = %v", first["id"])
	}
	if columns[1].(map[string]any)["id"] != "r0c1" {
		t.Errorf("auto id = %v, want r0c1", columns[1].(map[string]any)["id"])
	}

	// Non-admin: both gated columns drop, and now BOTH rows vanish.
	limited := &model.User{Username: "u", Roles: []string{}}
	doc = normalizePageDocument(map[string]any{
		"rows": []any{
			map[string]any{"columns": []any{map[string]any{"id": "a", "type": "stats", "handler": "h"}}},
			map[string]any{"columns": []any{map[string]any{"id": "b", "type": "table", "handler": "h", "permission": "read"}}},
		},
	}, limited, demoPlugin())
	if got := len(doc["rows"].([]any)); got != 1 {
		t.Errorf("rows for limited user = %d, want 1", got)
	}

	// Malformed payloads become error documents.
	doc = normalizePageDocument([]any{"nope"}, pageAdminUser(), demoPlugin())
	if doc["error"] == nil {
		t.Errorf("malformed document should carry an error: %v", doc)
	}
	doc = normalizePageDocument(map[string]any{"rows": "nope"}, pageAdminUser(), demoPlugin())
	if doc["error"] == nil {
		t.Errorf("non-list rows should carry an error: %v", doc)
	}
}

// TestWritePluginJSONColumnPassthrough pins the table-payload bug: a
// column's data may carry "rows" (its own table rows) and must NOT be
// re-normalized as a page layout, which emptied it.
func TestWritePluginJSONColumnPassthrough(t *testing.T) {
	config.SetServerConfig(&config.ServerConfig{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/plugins/x/home?_json=1&_col=spaces", nil)
	writePluginJSON(w, map[string]any{
		"columns": []any{map[string]any{"key": "name", "label": "Space"}},
		"rows":    []any{map[string]any{"name": "alpha"}},
	}, pageAdminUser(), demoPlugin(), r)
	body := w.Body.String()
	if !strings.Contains(body, `"columns"`) || !strings.Contains(body, "alpha") {
		t.Errorf("column payload was mangled: %s", body)
	}

	// Without _col the same shape is treated as a layout (back-compat with
	// the page transport).
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("GET", "/plugins/x/home?_json=1", nil)
	writePluginJSON(w2, map[string]any{
		"rows": []any{map[string]any{"name": "alpha"}},
	}, pageAdminUser(), demoPlugin(), r2)
	if !strings.Contains(w2.Body.String(), `"rows":[]`) {
		t.Errorf("layout normalization should drop non-layout rows: %s", w2.Body.String())
	}
}
