package plugins

import (
	"strings"
	"testing"
)

// TestMCPToolDeclParsing pins the [[tool.knot.mcp_tools]] contract: gates
// parse and qualify, the name defaults to the handler, and malformed
// declarations fail the load loudly.
func TestMCPToolDeclParsing(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "tooler", `requires-scriptling = ">=0.24"

[tool.knot]
version = "1.0"
permissions = ["read"]

[[tool.knot.mcp_tools]]
description = "Echo a word."
handler = "echo_word"

[[tool.knot.mcp_tools]]
name = "admin_scan"
description = "Scan everything."
handler = "scan_all"
permission = "read"
groups = ["platform", "sre"]

[[tool.knot.mcp_tools.parameters]]
name = "hours"
type = "int"
description = "Hours to scan back."
default = 24

[[tool.knot.mcp_tools.parameters]]
name = "scope"
type = "string"
description = "What to scan."
required = true`)

	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if len(registry.Failed()) != 0 {
		t.Fatalf("failed = %+v", registry.Failed())
	}
	plugin := registry.ByName("tooler")
	if plugin == nil {
		t.Fatal("plugin not loaded")
	}
	if len(plugin.MCPTools) != 2 {
		t.Fatalf("mcp tools = %d, want 2", len(plugin.MCPTools))
	}
	if plugin.MCPTools[0].Name != "echo_word" {
		t.Errorf("default name = %q, want the handler name", plugin.MCPTools[0].Name)
	}
	if plugin.MCPTools[0].Permission != "" || len(plugin.MCPTools[0].Groups) != 0 {
		t.Errorf("ungated tool = %+v, want empty gates", plugin.MCPTools[0])
	}
	if plugin.MCPTools[1].Permission != "plugin.tooler.read" || len(plugin.MCPTools[1].Groups) != 2 || plugin.MCPTools[1].Groups[0] != "platform" {
		t.Errorf("gated tool = %+v, want qualified permission and groups", plugin.MCPTools[1])
	}
	params := plugin.MCPTools[1].Parameters
	if len(params) != 2 {
		t.Fatalf("parameters = %d, want 2", len(params))
	}
	if params[0].Name != "hours" || params[0].Type != "int" || params[0].Default == nil || params[0].Required {
		t.Errorf("hours param = %+v", params[0])
	}
	if params[1].Name != "scope" || params[1].Type != "string" || !params[1].Required {
		t.Errorf("scope param = %+v", params[1])
	}
}

func TestMCPToolDeclValidation(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		match string
	}{
		{"unknown key", `
[tool.knot]
version = "1.0"

[[tool.knot.mcp_tools]]
handler = "x"
description = "X."
extra = true`, "unknown key"},
		{"missing handler", `
[tool.knot]
version = "1.0"

[[tool.knot.mcp_tools]]
description = "X."`, "handler is required"},
		{"missing description", `
[tool.knot]
version = "1.0"

[[tool.knot.mcp_tools]]
handler = "x"`, "description is required"},
		{"bad name", `
[tool.knot]
version = "1.0"

[[tool.knot.mcp_tools]]
name = "has space"
handler = "x"
description = "X."`, "name must match"},
		{"undeclared permission", `
[tool.knot]
version = "1.0"

[[tool.knot.mcp_tools]]
handler = "x"
description = "X."
permission = "other"`, "not declared"},
		{"duplicate in plugin", `
[tool.knot]
version = "1.0"

[[tool.knot.mcp_tools]]
handler = "x"
description = "X."

[[tool.knot.mcp_tools]]
name = "x"
handler = "y"
description = "Y."`, "duplicate tool name"},
		{"bad parameter type", `
[tool.knot]
version = "1.0"

[[tool.knot.mcp_tools]]
handler = "x"
description = "X."

[[tool.knot.mcp_tools.parameters]]
name = "p"
type = "datetime"`, "type must be one of"},
		{"missing parameter name", `
[tool.knot]
version = "1.0"

[[tool.knot.mcp_tools]]
handler = "x"
description = "X."

[[tool.knot.mcp_tools.parameters]]
type = "int"`, "name is required"},
		{"duplicate parameter", `
[tool.knot]
version = "1.0"

[[tool.knot.mcp_tools]]
handler = "x"
description = "X."

[[tool.knot.mcp_tools.parameters]]
name = "p"

[[tool.knot.mcp_tools.parameters]]
name = "p"`, "duplicate parameter"},
		{"parameter unknown key", `
[tool.knot]
version = "1.0"

[[tool.knot.mcp_tools]]
handler = "x"
description = "X."

[[tool.knot.mcp_tools.parameters]]
name = "p"
unit = "hours"`, "unknown key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := loadFailing(t, "broken", tc.body)
			if !strings.Contains(reason, tc.match) {
				t.Fatalf("reason = %q, want %q", reason, tc.match)
			}
		})
	}
}

// TestMCPToolNameCrossPluginUniqueness pins the shared namespace: two
// plugins exposing the same tool name cannot both load — the first by
// plugin name wins, the later fails with the collision named.
func TestMCPToolNameCrossPluginUniqueness(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "aaa", `requires-scriptling = ">=0.24"

[tool.knot]
version = "1.0"

[[tool.knot.mcp_tools]]
handler = "shared"
description = "First."`)
	writePlugin(t, dir, "bbb", `requires-scriptling = ">=0.24"

[tool.knot]
version = "1.0"

[[tool.knot.mcp_tools]]
handler = "other"
name = "shared"
description = "Second."`)

	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if registry.ByName("aaa") == nil {
		t.Fatal("first plugin should load")
	}
	failed := registry.Failed()
	if len(failed) != 1 || failed[0].Name != "bbb" || !strings.Contains(failed[0].Reason, "already exposed by plugin aaa") {
		t.Fatalf("failed = %+v, want bbb failing with the collision named", failed)
	}
}
