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
		{Name: "env", Type: "autocomplete", Handler: "plugin.demo-scriptling.field_environment"},
		{Name: "config", Type: "textarea", Language: "yaml"},
	})
	if errMsg != "" {
		t.Fatalf("valid fields rejected: %s", errMsg)
	}
	if len(fields) != 5 {
		t.Fatalf("fields = %d, want 5", len(fields))
	}
	if fields[0].Type != "text" || fields[0].Handler != "" || fields[0].Language != "" {
		t.Errorf("defaults = %+v, want type text with no extras", fields[0])
	}
	if fields[3].Handler != "plugin.demo-scriptling.field_environment" {
		t.Errorf("handler = %q", fields[3].Handler)
	}
	if fields[4].Language != "yaml" {
		t.Errorf("language = %q", fields[4].Language)
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
