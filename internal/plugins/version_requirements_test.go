package plugins

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/build"
)

// loadFailing loads a one-plugin dir and returns the recorded failure reason.
func loadFailing(t *testing.T, name, metadataBody string) string {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, name, metadataBody)
	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	failed := registry.Failed()
	if len(failed) != 1 {
		t.Fatalf("failed = %+v, want exactly one failure", failed)
	}
	return failed[0].Reason
}

// TestRequiresScriptlingTooNew pins the version gate: a plugin requiring a
// host newer than the running one fails to load with a reason naming the
// requirement. In knot's embedding the host version is knot's own build
// version — from a plugin's perspective the host is the interpreter it
// runs on.
func TestRequiresScriptlingTooNew(t *testing.T) {
	reason := loadFailing(t, "toonew", `requires-scriptling = ">=10.0.0"

[tool.knot]
version = "1.0"`)
	if !strings.Contains(reason, "10.0.0") || !strings.Contains(reason, build.Version) {
		t.Fatalf("reason = %q, want the requirement %q and host version %q named", reason, ">=10.0.0", build.Version)
	}
}

// TestRequiresScriptlingSatisfied pins the happy side: the current host
// version satisfies an exact lower bound.
func TestRequiresScriptlingSatisfied(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "oldenough", `requires-scriptling = ">=0.1"

[tool.knot]
version = "1.0"`)
	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if len(registry.Failed()) != 0 {
		t.Fatalf("failed = %+v, want a clean load", registry.Failed())
	}
}

// TestUnknownLibraryDependency pins the dependency gate: a required library
// no loader can produce is a load failure naming it.
func TestUnknownLibraryDependency(t *testing.T) {
	reason := loadFailing(t, "wantslib", `requires-scriptling = ">=0.1"
dependencies = ["no_such_library_anywhere"]

[tool.knot]
version = "1.0"`)
	if !strings.Contains(reason, "no_such_library_anywhere") || !strings.Contains(reason, "not available") {
		t.Fatalf("reason = %q, want the library named as unavailable", reason)
	}
}

// TestForeignPeerDependencyFails pins cross-plugin isolation at the load
// gate: a plugin declaring another plugin's peer as a dependency (a bin/
// peer it does not ship itself) fails to load — peers are components of
// their plugin, never shared libraries.
func TestForeignPeerDependencyFails(t *testing.T) {
	reason := loadFailing(t, "borrower", `requires-scriptling = ">=0.1"
dependencies = ["plugin.demolib via demolib >= 1.0.0"]

[tool.knot]
version = "1.0"`)
	if !strings.Contains(reason, "demolib") || !strings.Contains(reason, "not loaded") {
		t.Fatalf("reason = %q, want the peer named as not loaded", reason)
	}
}
