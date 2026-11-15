package mcptools

import (
	"reflect"
	"testing"

	"github.com/paularlott/lmchatkit"
)

// requires_approval and always_confirm are published as destructiveHint, so
// MCP clients ask the user before each call; an explicit destructive key
// wins over both.
func TestParseMetadataApprovalIsDestructive(t *testing.T) {
	cases := []struct {
		name string
		toml string
		want *bool
	}{
		{"requires approval", "description = \"d\"\nrequires_approval = true\n", ptr(true)},
		{"older name", "description = \"d\"\nrequires_permission = true\n", ptr(true)},
		{"approval off", "description = \"d\"\nrequires_approval = false\n", nil},
		{"no flag", "description = \"d\"\n", nil},
		{"always confirm", "description = \"d\"\nalways_confirm = true\n", ptr(true)},
		{"always confirm on write", "description = \"d\"\nalways_confirm = { mode = \"write\" }\n", ptr(true)},
		{"explicit destructive wins", "description = \"d\"\nrequires_approval = true\ndestructive = false\n", ptr(false)},
		{"destructive alone", "description = \"d\"\ndestructive = true\n", ptr(true)},
	}
	for _, c := range cases {
		meta, err := ParseMetadata([]byte(c.toml))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !samePtr(meta.Destructive, c.want) {
			t.Errorf("%s: Destructive = %v, want %v", c.name, show(meta.Destructive), show(c.want))
		}
	}
}

func TestParseAlwaysConfirm(t *testing.T) {
	cases := []struct {
		toml string
		want any
	}{
		{"", nil},
		{"always_confirm = false\n", nil},
		{"requires_approval = true\n", nil},
		{"always_confirm = true\n", true},
		{"always_confirm = {}\n", true},
		{"always_confirm = { mode = \"write\" }\n", map[string]any{"mode": "write"}},
	}
	for _, c := range cases {
		if got := ParseAlwaysConfirm([]byte(c.toml)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %#v, want %#v", c.toml, got, c.want)
		}
	}
}

// User script tools carry requires_approval and always_confirm in their
// schema TOML: requires_approval is only the destructive hint, so the web
// assistant still offers "Always Allow"; always_confirm adds the rule.
func TestToolHints(t *testing.T) {
	params := "\n[[parameters]]\nname = \"x\"\ntype = \"string\"\n"
	if a, m := ToolHints(""); a != nil || m != nil {
		t.Errorf("empty TOML: %+v %+v", a, m)
	}
	if a, m := ToolHints(params); a != nil || m != nil {
		t.Errorf("no flag: %+v %+v", a, m)
	}
	a, m := ToolHints("requires_approval = true\n" + params)
	if a == nil || a.DestructiveHint == nil || !*a.DestructiveHint || m != nil {
		t.Errorf("requires_approval: %+v %+v", a, m)
	}
	a, m = ToolHints("always_confirm = true\n" + params)
	if a == nil || a.DestructiveHint == nil || !*a.DestructiveHint || m[lmchatkit.AlwaysConfirmMetaKey] != true {
		t.Errorf("always_confirm: %+v %+v", a, m)
	}
}

// The built-in tools that ask for approval are listed as destructive, with no
// always-confirm rule (the web assistant offers "Always Allow" for them);
// run_script's write-mode runs are always confirmed.
func TestBuiltinToolApprovalHints(t *testing.T) {
	mustLoadTools(t)
	byName := map[string]bool{}
	rules := map[string]any{}
	for _, tool := range GetAllMCPTools() {
		byName[tool.Name] = tool.Annotations != nil && tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint
		if rule, ok := tool.Meta[lmchatkit.AlwaysConfirmMetaKey]; ok {
			rules[tool.Name] = rule
		}
	}
	for _, name := range []string{"delete_space", "run_command", "write_file", "stop_space", "run_space_script", "run_script"} {
		if !byName[name] {
			t.Errorf("%s is not listed as destructive", name)
		}
	}
	for _, name := range []string{"create_space", "start_space", "list_spaces", "script_docs"} {
		if byName[name] {
			t.Errorf("%s is listed as destructive", name)
		}
	}
	if want := map[string]any{"run_script": map[string]any{"mode": "write"}}; !reflect.DeepEqual(rules, want) {
		t.Errorf("always-confirm rules = %#v, want %#v", rules, want)
	}

	// The rule is what lmchatkit applies: write runs are picked, read runs not.
	rule := lmchatkit.ConfirmRule(rules["run_script"].(map[string]any))
	if !rule.Matches([]byte(`{"code":"x","mode":"write"}`)) || rule.Matches([]byte(`{"code":"x","mode":"read"}`)) || rule.Matches([]byte(`{"code":"x"}`)) {
		t.Error("run_script's rule does not pick exactly the write-mode runs")
	}
}

func ptr(b bool) *bool { return &b }

func samePtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func show(b *bool) string {
	if b == nil {
		return "unset"
	}
	if *b {
		return "true"
	}
	return "false"
}
