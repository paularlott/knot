package mcp

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
)

// pluginToolFixture loads a one-plugin registry exposing two MCP tools: one
// ungated, one permission-gated — whose handler also reports the identity
// surface (request.user) the dispatch provides.
func pluginToolFixture(t *testing.T) {
	t.Helper()
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	model.SetRoleCache(nil)

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "tooler")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `# /// script
# requires-scriptling = ">=0.24"
#
# [tool.knot]
# version = "1.0"
# permissions = ["read"]
#
# [[tool.knot.mcp_tools]]
# name = "echo_word"
# description = "Echo a word."
# handler = "echo_word"
#
# [[tool.knot.mcp_tools.parameters]]
# name = "word"
# type = "string"
# description = "The word to echo."
# required = true
#
# [[tool.knot.mcp_tools]]
# name = "admin_scan"
# description = "Scan everything."
# handler = "scan_all"
# permission = "read"
#
# [[tool.knot.mcp_tools]]
# name = "self_guarded"
# description = "Guards itself in code."
# handler = "self_guarded"
# ///
def echo_word():
    import scriptling.mcp.tool as tool

    word = tool.get_string("word", "")
    tool.return_object({"reply": "echo: " + word.upper(), "as_user": user.name, "is_admin": user.is_admin})


def scan_all():
    import scriptling.mcp.tool as tool

    if not user.has_permission("manage_spaces"):
        tool.return_error("admin only")
    tool.return_string("scanned")


def self_guarded():
    import scriptling.mcp.tool as tool

    if not user.is_admin:
        tool.return_error("self guard: admins only")
    tool.return_string("self guarded ok")
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

// TestPluginMCPToolListing pins the listing gate: ungated tools appear for
// any MCP user, permission-gated tools only for users who pass the grant.
func TestPluginMCPToolListing(t *testing.T) {
	pluginToolFixture(t)
	ctx := context.Background()

	plain := &model.User{Username: "plain", Id: "u-plain"}
	tools, err := NewScriptToolsProvider(plain).GetTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	var echoSchema map[string]interface{}
	for _, tool := range tools {
		if tool.Name == "echo_word" {
			echoSchema, _ = tool.InputSchema.(map[string]interface{})
		}
	}
	if echoSchema == nil {
		t.Fatalf("echo_word missing from listing: %v", names)
	}
	props, _ := echoSchema["properties"].(map[string]interface{})
	word, _ := props["word"].(map[string]interface{})
	if word == nil || word["type"] != "string" || word["description"] != "The word to echo." {
		t.Errorf("echo_word schema properties = %v, want the declared word parameter", props)
	}
	if req, _ := echoSchema["required"].([]string); len(req) != 1 || req[0] != "word" {
		t.Errorf("echo_word required = %v, want [word]", echoSchema["required"])
	}
	if names["admin_scan"] {
		t.Errorf("gated plugin tool listed for a user without the grant: %v", names)
	}

	admin := &model.User{Username: "admin", Id: "u-admin", Roles: []string{model.RoleAdminUUID}}
	tools, err = NewScriptToolsProvider(admin).GetTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names = map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if !names["echo_word"] || !names["admin_scan"] {
		t.Errorf("admin listing missing plugin tools: %v", names)
	}
}

// TestPluginMCPToolExecute pins execution: the tool dispatches the plugin
// handler as the requesting user (params arrive, the request.user identity
// surface is populated), gated tools refuse ungranted users, and unknown
// names stay not-found for other providers.
func TestPluginMCPToolExecute(t *testing.T) {
	pluginToolFixture(t)
	ctx := context.Background()

	plain := &model.User{Username: "plain", Id: "u-plain"}
	provider := NewScriptToolsProvider(plain)

	// Ungated tool: runs the handler with the caller's params and identity.
	resp, err := provider.ExecuteTool(ctx, "echo_word", map[string]interface{}{"word": "hi"})
	if err != nil {
		t.Fatalf("echo_word: %v", err)
	}
	if resp == nil {
		t.Fatal("echo_word: nil response")
	}
	var body strings.Builder
	for _, c := range resp.Content {
		body.WriteString(c.Text)
	}
	for _, want := range []string{"echo: HI", "plain"} {
		if !strings.Contains(body.String(), want) {
			t.Errorf("echo_word response missing %q: %s", want, body.String())
		}
	}

	// Gated tool: refused for a user without the grant.
	if _, err := provider.ExecuteTool(ctx, "admin_scan", map[string]interface{}{}); err == nil {
		t.Fatal("admin_scan: want permission error for plain user")
	}

	// Gated tool: admin passes.
	admin := &model.User{Username: "admin", Id: "u-admin", Roles: []string{model.RoleAdminUUID}}
	if _, err := NewScriptToolsProvider(admin).ExecuteTool(ctx, "admin_scan", map[string]interface{}{}); err != nil {
		t.Fatalf("admin_scan as admin: %v", err)
	}

	// A tool with no metadata gate that self-checks in code: the
	// return_error path surfaces as the tool's error.
	if _, err := provider.ExecuteTool(ctx, "self_guarded", map[string]interface{}{}); err == nil || !strings.Contains(err.Error(), "self guard") {
		t.Fatalf("self_guarded as plain: err = %v, want the in-code guard's error", err)
	}
	if resp, err := NewScriptToolsProvider(admin).ExecuteTool(ctx, "self_guarded", map[string]interface{}{}); err != nil || resp == nil {
		t.Fatalf("self_guarded as admin: resp = %v, err = %v", resp, err)
	}

	// Unknown name: not-found, so other providers get their turn.
	resp, err = provider.ExecuteTool(ctx, "no_such_tool", map[string]interface{}{})
	if resp != nil || err != nil {
		t.Fatalf("unknown tool: resp = %v, err = %v, want nil/nil", resp, err)
	}
}
