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
)

// TestPluginInventoryComplete pins the admin inventory: every declaration
// surface appears in /api/plugins — MCP tools (with parameters and gates),
// ajax handlers, field handlers and scriptling peers alongside the
// permissions, menus, pages and binary peers that predate them.
func TestPluginInventoryComplete(t *testing.T) {
	config.SetServerConfig(&config.ServerConfig{})
	model.SetRoleCache(nil)

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "full")
	peersDir := filepath.Join(pluginDir, "peers")
	if err := os.MkdirAll(peersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `# /// script
# requires-scriptling = ">=0.24"
# dependencies = [
#   "plugin.calc via calc >= 1.0.0",
# ]
#
# [tool.knot]
# version = "1.0"
# permissions = ["read"]
#
# [[tool.knot.pages]]
# path = "/home"
# handler = "home"
#
# [[tool.knot.handlers]]
# handler = "save"
# permission = "read"
#
# [[tool.knot.mcp_tools]]
# name = "export"
# description = "Export things."
# handler = "export"
# permission = "read"
# groups = ["platform"]
#
# [[tool.knot.mcp_tools.parameters]]
# name = "hours"
# type = "int"
# default = 24
#
# [[tool.knot.field_handlers]]
# label = "Env"
# handler = "field_env"
# ///
def home():
    return {}


def save():
    return {}


def export():
    return {}


def field_env():
    return {"options": []}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(peersDir, "calc.py"), []byte("def add(a, b):\n    return a + b\n"), 0o644); err != nil {
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

	r := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	r = r.WithContext(context.WithValue(r.Context(), "user", &model.User{Username: "admin", Roles: []string{model.RoleAdminUUID}}))
	w := httptest.NewRecorder()
	HandleGetPlugins(w, r)

	var list struct {
		Plugins []struct {
			MCPTools []struct {
				Name       string   `json:"name"`
				Permission string   `json:"permission"`
				Groups     []string `json:"groups"`
				Parameters []string `json:"parameters"`
			} `json:"mcp_tools"`
			Handlers []struct {
				Handler    string `json:"handler"`
				Permission string `json:"permission"`
			} `json:"handlers"`
			FieldHandlers []struct {
				Id    string `json:"id"`
				Label string `json:"label"`
			} `json:"field_handlers"`
			ScriptPeers []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"script_peers"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(list.Plugins) != 1 {
		t.Fatalf("plugins = %d, want 1", len(list.Plugins))
	}
	p := list.Plugins[0]

	if len(p.MCPTools) != 1 || p.MCPTools[0].Name != "export" {
		t.Fatalf("mcp tools = %+v", p.MCPTools)
	}
	if p.MCPTools[0].Permission != "plugin.full.read" || len(p.MCPTools[0].Groups) != 1 {
		t.Errorf("mcp tool gates = %+v", p.MCPTools[0])
	}
	if len(p.MCPTools[0].Parameters) != 1 || p.MCPTools[0].Parameters[0] != "hours" {
		t.Errorf("mcp tool parameters = %v", p.MCPTools[0].Parameters)
	}
	if len(p.Handlers) != 1 || p.Handlers[0].Handler != "save" || p.Handlers[0].Permission != "plugin.full.read" {
		t.Errorf("handlers = %+v", p.Handlers)
	}
	if len(p.FieldHandlers) != 1 || p.FieldHandlers[0].Id != "plugin.full.field_env" {
		t.Errorf("field handlers = %+v", p.FieldHandlers)
	}
	if len(p.ScriptPeers) != 1 || p.ScriptPeers[0].Name != "calc" || p.ScriptPeers[0].Version != "1.0" {
		t.Errorf("script peers = %+v", p.ScriptPeers)
	}
}
