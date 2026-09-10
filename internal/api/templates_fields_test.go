package api

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

// TestNormalizeCustomFields pins the custom-field contract on template
// create/update: types are the set the space form renders, autocomplete
// handlers carry the qualified plugin id shape, textarea languages are the
// editor's set — and the pairs that do not apply are cleared, not stored.
func TestNormalizeCustomFields(t *testing.T) {
	// The full happy path: every type with its applicable extras.
	fields, errMsg := normalizeCustomFields([]apiclient.CustomFieldDef{
		{Name: "plain"},
		{Name: "secret", Type: "masked"},
		{Name: "count", Type: "number"},
		{Name: "debug", Type: "bool", Default: "true"},
		{Name: "env2", Type: "select", Handler: "plugin.demo-scriptling.field_environment"},
		{Name: "env", Type: "autocomplete", Handler: "plugin.demo-scriptling.field_environment"},
		{Name: "config", Type: "textarea", Language: "yaml"},
	})
	if errMsg != "" {
		t.Fatalf("valid fields rejected: %s", errMsg)
	}
	if len(fields) != 7 {
		t.Fatalf("fields = %d, want 7", len(fields))
	}
	if fields[0].Type != "text" || fields[0].Handler != "" || fields[0].Language != "" {
		t.Errorf("defaults = %+v, want type text with no extras", fields[0])
	}
	// A bool field rides along with its "true"/"false" string default and
	// no handler/language, like every non-applicable pair.
	if fields[3].Type != "bool" || fields[3].Default != "true" || fields[3].Handler != "" || fields[3].Language != "" {
		t.Errorf("bool field = %+v", fields[3])
	}
	if fields[4].Type != "select" || fields[4].Handler != "plugin.demo-scriptling.field_environment" {
		t.Errorf("select field = %+v, want handler kept", fields[4])
	}
	if fields[5].Handler != "plugin.demo-scriptling.field_environment" {
		t.Errorf("handler = %q", fields[5].Handler)
	}
	if fields[6].Language != "yaml" {
		t.Errorf("language = %q", fields[6].Language)
	}

	// Defaults ride along verbatim, on every type.
	fields, errMsg = normalizeCustomFields([]apiclient.CustomFieldDef{
		{Name: "branch", Default: "main"},
		{Name: "secret", Type: "masked", Default: "hunter2"},
		{Name: "config", Type: "textarea", Language: "yaml", Default: "key: value\n"},
	})
	if errMsg != "" {
		t.Fatalf("fields with defaults rejected: %s", errMsg)
	}
	if fields[0].Default != "main" || fields[1].Default != "hunter2" || fields[2].Default != "key: value\n" {
		t.Errorf("defaults = %q / %q / %q, want them stored verbatim", fields[0].Default, fields[1].Default, fields[2].Default)
	}

	// A bool's stray extras are cleared like any other type's; "boolean"
	// and "true"/"false" strings are not aliases — the type is "bool".
	fields, errMsg = normalizeCustomFields([]apiclient.CustomFieldDef{
		{Name: "x", Type: "bool", Handler: "plugin.demo.field", Language: "yaml", Default: "false"},
	})
	if errMsg != "" {
		t.Fatalf("bool with stray extras rejected: %s", errMsg)
	}
	if fields[0].Handler != "" || fields[0].Language != "" || fields[0].Default != "false" {
		t.Errorf("bool stray extras stored: %+v, want cleared and default kept", fields[0])
	}

	// Required rides along on every type.
	fields, errMsg = normalizeCustomFields([]apiclient.CustomFieldDef{
		{Name: "branch", Required: true},
		{Name: "debug", Type: "bool", Required: true, Default: "true"},
	})
	if errMsg != "" {
		t.Fatalf("required fields rejected: %s", errMsg)
	}
	if !fields[0].Required || !fields[1].Required {
		t.Errorf("required = %v / %v, want both true", fields[0].Required, fields[1].Required)
	}
	if _, errMsg := normalizeCustomFields([]apiclient.CustomFieldDef{{Name: "x", Type: "boolean"}}); errMsg == "" || !strings.Contains(errMsg, "custom_fields[0].type") {
		t.Errorf("\"boolean\" should be rejected (the type is \"bool\"): errMsg = %q", errMsg)
	}

	// Unknown types are load errors, not silent text-field downgrades.
	if _, errMsg := normalizeCustomFields([]apiclient.CustomFieldDef{{Name: "x", Type: "colour"}}); errMsg == "" || !strings.Contains(errMsg, "custom_fields[0].type") {
		t.Errorf("unknown type: errMsg = %q, want a type error", errMsg)
	}

	// Autocomplete requires a well-formed handler id; module.function
	// handlers are qualified ids too.
	if _, errMsg := normalizeCustomFields([]apiclient.CustomFieldDef{{Name: "x", Type: "autocomplete"}}); errMsg == "" || !strings.Contains(errMsg, "handler") {
		t.Errorf("missing handler: errMsg = %q, want a handler error", errMsg)
	}
	// select draws its options from a handler too: same requirement.
	if _, errMsg := normalizeCustomFields([]apiclient.CustomFieldDef{{Name: "x", Type: "select"}}); errMsg == "" || !strings.Contains(errMsg, "handler") {
		t.Errorf("select without handler: errMsg = %q, want a handler error", errMsg)
	}

	// A manual option list is select's other source — exactly one of the
	// two, never both, and options are trimmed of blank lines.
	fields, errMsg = normalizeCustomFields([]apiclient.CustomFieldDef{
		{Name: "size", Type: "select", Options: []string{" small ", "", "large", ""}},
	})
	if errMsg != "" {
		t.Fatalf("select with manual options rejected: %s", errMsg)
	}
	if len(fields[0].Options) != 2 || fields[0].Options[0] != "small" || fields[0].Options[1] != "large" || fields[0].Handler != "" {
		t.Errorf("manual options = %+v, want trimmed [small large] with no handler", fields[0])
	}
	if _, errMsg := normalizeCustomFields([]apiclient.CustomFieldDef{{Name: "x", Type: "select", Handler: "plugin.demo.h", Options: []string{"a"}}}); errMsg == "" || !strings.Contains(errMsg, "not both") {
		t.Errorf("select with both sources: errMsg = %q", errMsg)
	}
	if _, errMsg := normalizeCustomFields([]apiclient.CustomFieldDef{{Name: "x", Type: "select", Options: []string{"  "}}}); errMsg == "" || !strings.Contains(errMsg, "options") {
		t.Errorf("select with blank-only options: errMsg = %q", errMsg)
	}
	// Autocomplete takes the same two sources as select — including a
	// manual list — and rejects mixing them.
	fields, errMsg = normalizeCustomFields([]apiclient.CustomFieldDef{
		{Name: "x", Type: "autocomplete", Options: []string{"dev", "prod"}},
	})
	if errMsg != "" {
		t.Fatalf("autocomplete with manual options rejected: %s", errMsg)
	}
	if len(fields[0].Options) != 2 || fields[0].Handler != "" {
		t.Errorf("autocomplete manual options = %+v", fields[0])
	}
	if _, errMsg := normalizeCustomFields([]apiclient.CustomFieldDef{{Name: "x", Type: "autocomplete", Handler: "plugin.demo.h", Options: []string{"a"}}}); errMsg == "" || !strings.Contains(errMsg, "not both") {
		t.Errorf("autocomplete with both sources: errMsg = %q", errMsg)
	}
	// Options are cleared on the types that don't take them.
	fields, errMsg = normalizeCustomFields([]apiclient.CustomFieldDef{
		{Name: "y", Options: []string{"a"}},
	})
	if errMsg != "" {
		t.Fatalf("options on other types rejected: %s", errMsg)
	}
	if fields[0].Options != nil {
		t.Errorf("options not cleared: %+v", fields[0])
	}
	if _, errMsg := normalizeCustomFields([]apiclient.CustomFieldDef{{Name: "x", Type: "autocomplete", Handler: "field_environment"}}); errMsg == "" {
		t.Errorf("unqualified handler accepted: %q", errMsg)
	}
	if _, errMsg := normalizeCustomFields([]apiclient.CustomFieldDef{{Name: "x", Type: "autocomplete", Handler: "plugin.demo.mod.fn"}}); errMsg != "" {
		t.Errorf("module.function handler rejected: %s", errMsg)
	}

	// Language is validated on textarea and cleared elsewhere.
	if _, errMsg := normalizeCustomFields([]apiclient.CustomFieldDef{{Name: "x", Type: "textarea", Language: "perl"}}); errMsg == "" || !strings.Contains(errMsg, "language") {
		t.Errorf("unknown language: errMsg = %q, want a language error", errMsg)
	}
	fields, errMsg = normalizeCustomFields([]apiclient.CustomFieldDef{
		{Name: "x", Type: "text", Handler: "plugin.demo.field", Language: "yaml"},
	})
	if errMsg != "" {
		t.Fatalf("stray extras rejected: %s", errMsg)
	}
	if fields[0].Handler != "" || fields[0].Language != "" {
		t.Errorf("stray extras stored: %+v, want cleared", fields[0])
	}

	// A manual option list's default must be one of the options — the
	// value could never pass validation at create time. Handler-backed
	// fields can't be checked here (options are per request).
	fields, errMsg = normalizeCustomFields([]apiclient.CustomFieldDef{
		{Name: "size", Type: "select", Options: []string{"small", "large"}, Default: "large"},
	})
	if errMsg != "" {
		t.Fatalf("default matching an option rejected: %s", errMsg)
	}
	if fields[0].Default != "large" {
		t.Errorf("default = %q, want it kept", fields[0].Default)
	}
	if _, errMsg := normalizeCustomFields([]apiclient.CustomFieldDef{{Name: "size", Type: "select", Options: []string{"small", "large"}, Default: "medium"}}); errMsg == "" || !strings.Contains(errMsg, "custom_fields[0].default") {
		t.Errorf("default not an option: errMsg = %q, want a default error", errMsg)
	}
	fields, errMsg = normalizeCustomFields([]apiclient.CustomFieldDef{
		{Name: "env", Type: "autocomplete", Handler: "plugin.demo.h", Default: "anything"},
	})
	if errMsg != "" {
		t.Fatalf("handler-backed default rejected: %s", errMsg)
	}
	if fields[0].Default != "anything" {
		t.Errorf("handler-backed default = %q, want it kept for create-time validation", fields[0].Default)
	}
}
