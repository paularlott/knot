package plugins

import (
	"strings"
	"testing"
)

// TestAPIGeneration pins the plugin-system generation marker: api is
// optional and defaults to 1, api = 1 loads, and a plugin declaring a
// generation this knot doesn't implement is rejected loudly with a message
// that names both generations — the guarantee that a future api 2 can ship
// without old knots silently mis-parsing its plugins.
func TestAPIGeneration(t *testing.T) {
	// Absent api: the default generation.
	dir := t.TempDir()
	writePlugin(t, dir, "defaults", `
[tool.knot]
version = "1.0.0"
description = "no api declared"`)
	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p := registry.ByName("defaults"); p == nil || p.APIVersion != 1 {
		t.Fatalf("absent api should default to 1, got %+v", p)
	}

	// api = 1: today's generation, accepted and recorded.
	dir = t.TempDir()
	writePlugin(t, dir, "gen1", `
[tool.knot]
api = 1
version = "1.0.0"
description = "explicit generation"`)
	registry, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p := registry.ByName("gen1"); p == nil || p.APIVersion != 1 {
		t.Fatalf("api = 1 should load as generation 1, got %+v", p)
	}

	// api = 2: a generation this knot does not implement — a clear refusal,
	// not an incidental parse error.
	dir = t.TempDir()
	writePlugin(t, dir, "gen2", `
[tool.knot]
api = 2
version = "1.0.0"
description = "future generation"`)
	registry, _ = Load(dir)
	var reason string
	for _, f := range registry.Failed() {
		if f.Name == "gen2" {
			reason = f.Reason
		}
	}
	if reason == "" {
		t.Fatalf("api = 2 plugin should be failed, not loaded: %+v", registry.All())
	}
	if !strings.Contains(reason, "api 2 is not supported") || !strings.Contains(reason, "plugin api 1") {
		t.Fatalf("refusal should name both generations, got %q", reason)
	}

	// A non-integer api is a validation error.
	dir = t.TempDir()
	writePlugin(t, dir, "genbad", `
[tool.knot]
api = "1"
version = "1.0.0"
description = "wrong type"`)
	registry, _ = Load(dir)
	found := false
	for _, f := range registry.Failed() {
		if f.Name == "genbad" {
			found = true
		}
	}
	if !found {
		t.Fatalf("non-integer api should fail the plugin, got %+v", registry.All())
	}
}
