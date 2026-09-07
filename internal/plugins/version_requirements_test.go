package plugins

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/build"
)

// pinScriptlingVersion points requires-scriptling checking at a fixed
// embedded runtime version for the test, restoring the real source after.
func pinScriptlingVersion(t *testing.T, v string) {
	t.Helper()
	old := scriptlingVersionForVerify
	scriptlingVersionForVerify = func() string { return v }
	t.Cleanup(func() { scriptlingVersionForVerify = old })
}

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
// scriptling runtime newer than the embedded one fails to load with a
// reason naming the requirement and the embedded version.
func TestRequiresScriptlingTooNew(t *testing.T) {
	pinScriptlingVersion(t, "0.24.3")
	reason := loadFailing(t, "toonew", `requires-scriptling = ">=10.0.0"

[tool.knot]
version = "1.0"`)
	if !strings.Contains(reason, "10.0.0") || !strings.Contains(reason, "0.24.3") {
		t.Fatalf("reason = %q, want the requirement and the embedded version 0.24.3 named", reason)
	}
}

// TestRequiresScriptlingSatisfied pins the happy side: the embedded runtime
// satisfies the bound.
func TestRequiresScriptlingSatisfied(t *testing.T) {
	pinScriptlingVersion(t, "0.24.3")
	dir := t.TempDir()
	writePlugin(t, dir, "oldenough", `requires-scriptling = ">=0.24"

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

// TestRequiresScriptlingSkippedWithoutVersion pins the dev fallback: when
// the embedded runtime version cannot be determined (replace-directive dev
// builds, test binaries) the check is skipped, not a blanket failure.
func TestRequiresScriptlingSkippedWithoutVersion(t *testing.T) {
	pinScriptlingVersion(t, "unknown")
	dir := t.TempDir()
	writePlugin(t, dir, "nodevinfo", `requires-scriptling = ">=10.0.0"

[tool.knot]
version = "1.0"`)
	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if len(registry.Failed()) != 0 {
		t.Fatalf("failed = %+v, want the check skipped without an embedded version", registry.Failed())
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

// TestRequiresKnot pins the optional host bound: [tool.knot] requires_knot
// is checked against knot's own version at load, in both directions, and
// malformed constraints fail loudly.
func TestRequiresKnot(t *testing.T) {
	pinScriptlingVersion(t, "0.24.3")

	// Too new: the requirement and the running knot version are named.
	reason := loadFailing(t, "future", `requires-scriptling = ">=0.24"

[tool.knot]
version = "1.0"
requires_knot = ">=99.0"`)
	if !strings.Contains(reason, "99.0") || !strings.Contains(reason, build.Version) {
		t.Fatalf("reason = %q, want the constraint and knot version %q named", reason, build.Version)
	}

	// Satisfied: loads cleanly.
	dir := t.TempDir()
	writePlugin(t, dir, "present", `requires-scriptling = ">=0.24"

[tool.knot]
version = "1.0"
requires_knot = ">=0.1"`)
	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if len(registry.Failed()) != 0 {
		t.Fatalf("failed = %+v, want a clean load", registry.Failed())
	}

	// Malformed: a load error, not a silent pass.
	reason = loadFailing(t, "garbled", `requires-scriptling = ">=0.24"

[tool.knot]
version = "1.0"
requires_knot = "next tuesday"`)
	if !strings.Contains(reason, "cannot be checked") {
		t.Fatalf("reason = %q, want cannot-be-checked", reason)
	}
}
