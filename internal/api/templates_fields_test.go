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
		{Name: "env", Type: "autocomplete", Handler: "plugin.demo-scriptling.field_environment"},
		{Name: "config", Type: "textarea", Language: "yaml"},
	})
	if errMsg != "" {
		t.Fatalf("valid fields rejected: %s", errMsg)
	}
	if len(fields) != 6 {
		t.Fatalf("fields = %d, want 6", len(fields))
	}
	if fields[0].Type != "text" || fields[0].Handler != "" || fields[0].Language != "" {
		t.Errorf("defaults = %+v, want type text with no extras", fields[0])
	}
	// A bool field rides along with its "true"/"false" string default and
	// no handler/language, like every non-applicable pair.
	if fields[3].Type != "bool" || fields[3].Default != "true" || fields[3].Handler != "" || fields[3].Language != "" {
		t.Errorf("bool field = %+v", fields[3])
	}
	if fields[4].Handler != "plugin.demo-scriptling.field_environment" {
		t.Errorf("handler = %q", fields[4].Handler)
	}
	if fields[5].Language != "yaml" {
		t.Errorf("language = %q", fields[5].Language)
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
}
