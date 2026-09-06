package plugins

import (
	"strings"
	"testing"
)

// TestDefaultPage covers the post-login landing claim: one page per plugin,
// deterministic cross-plugin resolution (first plugin by name), and a warning
// when several plugins claim it.
func TestDefaultPage(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "beta", `# [tool.knot]
version = "1.0"

[[tool.knot.pages]]
path = "/home"
handler = "h"
default = true`)
	writePlugin(t, dir, "alpha", `# [tool.knot]
version = "1.0"

[[tool.knot.pages]]
path = "/landing"
handler = "h"
default = true`)
	writePlugin(t, dir, "gamma", `# [tool.knot]
version = "1.0"

[[tool.knot.pages]]
path = "/other"
handler = "h"`)

	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if len(registry.Failed()) != 0 {
		t.Fatalf("failed = %+v", registry.Failed())
	}

	if got := registry.DefaultPageURL(); got != "/plugins/alpha/landing" {
		t.Errorf("DefaultPageURL = %q, want the first claimant by name", got)
	}

	warned := false
	for _, w := range registry.Warnings() {
		if strings.Contains(w, "default page claimed by multiple plugins") && strings.Contains(w, "alpha wins") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("warnings = %v, want multi-claim warning", registry.Warnings())
	}

	// A second default page inside one plugin is a load error.
	dir2 := t.TempDir()
	writePlugin(t, dir2, "double", `# [tool.knot]
version = "1.0"

[[tool.knot.pages]]
path = "/a"
handler = "h"
default = true

[[tool.knot.pages]]
path = "/b"
handler = "h"
default = true`)
	registry2, err := Load(dir2)
	if err != nil {
		t.Fatal(err)
	}
	defer registry2.Close()
	if len(registry2.Failed()) != 1 || !strings.Contains(registry2.Failed()[0].Reason, "only one page per plugin") {
		t.Fatalf("failed = %+v", registry2.Failed())
	}
	if got := registry2.DefaultPageURL(); got != "" {
		t.Errorf("DefaultPageURL = %q, want empty", got)
	}
}
