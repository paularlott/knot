package plugins

import (
	"strings"
	"testing"
)

// TestHandlerDeclParsing pins the [[tool.knot.handlers]] contract: gates
// parse and qualify, and malformed declarations fail the load loudly.
func TestHandlerDeclParsing(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "gated", `# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0"
# permissions = ["read", "admin"]
#
# [[tool.knot.handlers]]
# handler = "export_all"
# permission = "admin"
#
# [[tool.knot.handlers]]
# handler = "mod.helper_fn"
#
# [[tool.knot.handlers]]
# handler = "ping"`)

	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if len(registry.Failed()) != 0 {
		t.Fatalf("failed = %+v", registry.Failed())
	}

	plugin := registry.ByName("gated")
	if plugin == nil {
		t.Fatal("plugin not loaded")
	}
	if decl := plugin.HandlerDecl("export_all"); decl == nil || decl.Permission != "plugin.gated.admin" {
		t.Errorf("export_all decl = %+v, want permission plugin.gated.admin", decl)
	}
	if decl := plugin.HandlerDecl("mod.helper_fn"); decl == nil || decl.Permission != "" {
		t.Errorf("mod.helper_fn decl = %+v, want an empty gate", decl)
	}
	if decl := plugin.HandlerDecl("ping"); decl == nil || decl.Permission != "" {
		t.Errorf("ping decl = %+v, want an empty gate", decl)
	}
	if decl := plugin.HandlerDecl("no_such"); decl != nil {
		t.Errorf("undeclared handler resolved: %+v", decl)
	}
}

func TestHandlerDeclValidation(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		match  string
	}{
		{"unknown key", `
[tool.knot]
version = "1.0"
permissions = ["read"]

[[tool.knot.handlers]]
handler = "x"
permissions = "read"`, "unknown key"},
		{"groups are gone from the plugin system", `
[tool.knot]
version = "1.0"

[[tool.knot.handlers]]
handler = "x"
groups = ["platform"]`, "unknown key"},
		{"undeclared permission", `
[tool.knot]
version = "1.0"
permissions = ["read"]

[[tool.knot.handlers]]
handler = "x"
permission = "other"`, "not declared"},
		{"duplicate handler", `
[tool.knot]
version = "1.0"

[[tool.knot.handlers]]
handler = "x"

[[tool.knot.handlers]]
handler = "x"`, "duplicate handler"},
		{"invalid handler name", `
[tool.knot]
version = "1.0"

[[tool.knot.handlers]]
handler = "has-dash"`, "must be a function name"},
		{"not a table", `
[tool.knot]
version = "1.0"
handlers = ["x"]`, "must be a table"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writePlugin(t, dir, "broken", tc.body)
			registry, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer registry.Close()
			if len(registry.Failed()) != 1 || !strings.Contains(registry.Failed()[0].Reason, tc.match) {
				t.Fatalf("failed = %+v, want reason matching %q", registry.Failed(), tc.match)
			}
		})
	}
}
