package portforward

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"
	"testing/quick"
)

// setTestPath sets the testConfigPath to a temp file path and returns a cleanup function.
func setTestPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "port-forward.toml")
	testConfigPath = &path
	t.Cleanup(func() { testConfigPath = nil })
	return path
}

// --- Property Tests ---

// Property 1: Persistence Roundtrip
// For any valid (localPort, remotePort, space), SaveForward then LoadForwards
// returns an entry matching all three fields.
// Validates: Requirements 1.1, 1.2, 1.4, 6.1, 6.2
func TestProperty_PersistenceRoundtrip(t *testing.T) {
	f := func(localPort, remotePort uint16, space string) bool {
		// Constrain to valid inputs
		if localPort == 0 || remotePort == 0 || space == "" {
			return true // skip invalid inputs
		}

		setTestPath(t)

		if err := SaveForward(localPort, remotePort, space); err != nil {
			t.Logf("SaveForward error: %v", err)
			return false
		}

		entries, err := LoadForwards()
		if err != nil {
			t.Logf("LoadForwards error: %v", err)
			return false
		}

		for _, e := range entries {
			if e.LocalPort == localPort {
				return e.RemotePort == remotePort && e.Space == space
			}
		}
		return false
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Error(err)
	}
}

// Property 2: No Duplicates
// After saving the same localPort twice with different values, LoadForwards
// returns exactly one entry for that local_port.
// Validates: Requirements 7.1, 7.2
func TestProperty_NoDuplicates(t *testing.T) {
	f := func(localPort uint16, remotePort1, remotePort2 uint16) bool {
		if localPort == 0 || remotePort1 == 0 || remotePort2 == 0 {
			return true
		}

		setTestPath(t)

		if err := SaveForward(localPort, remotePort1, "space-a"); err != nil {
			return false
		}
		if err := SaveForward(localPort, remotePort2, "space-b"); err != nil {
			return false
		}

		entries, err := LoadForwards()
		if err != nil {
			return false
		}

		count := 0
		for _, e := range entries {
			if e.LocalPort == localPort {
				count++
			}
		}
		return count == 1
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Error(err)
	}
}

// Property 3: Remove Completeness
// After RemoveForward(lp), LoadForwards contains no entry with LocalPort == lp.
// Validates: Requirements 3.1, 3.3
func TestProperty_RemoveCompleteness(t *testing.T) {
	f := func(localPort uint16, remotePort uint16) bool {
		if localPort == 0 || remotePort == 0 {
			return true
		}

		setTestPath(t)

		if err := SaveForward(localPort, remotePort, "test-space"); err != nil {
			return false
		}
		if err := RemoveForward(localPort); err != nil {
			return false
		}

		entries, err := LoadForwards()
		if err != nil {
			return false
		}

		for _, e := range entries {
			if e.LocalPort == localPort {
				return false
			}
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Error(err)
	}
}

// Property 4: Remove Preservation
// Save two entries with different local ports, RemoveForward one, verify the other is still present.
// Validates: Requirements 3.3
func TestProperty_RemovePreservation(t *testing.T) {
	f := func(port1, port2 uint16) bool {
		if port1 == 0 || port2 == 0 || port1 == port2 {
			return true
		}

		setTestPath(t)

		if err := SaveForward(port1, 1000, "space-1"); err != nil {
			return false
		}
		if err := SaveForward(port2, 2000, "space-2"); err != nil {
			return false
		}
		if err := RemoveForward(port1); err != nil {
			return false
		}

		entries, err := LoadForwards()
		if err != nil {
			return false
		}

		for _, e := range entries {
			if e.LocalPort == port2 {
				return true
			}
		}
		return false
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Error(err)
	}
}

// Property 7: Idempotent Remove
// Calling RemoveForward(lp) when no matching entry exists returns nil and does not corrupt the file.
// Validates: Requirements 3.2, 3.3
func TestProperty_IdempotentRemove(t *testing.T) {
	f := func(existingPort, removePort uint16) bool {
		if existingPort == 0 || removePort == 0 || existingPort == removePort {
			return true
		}

		setTestPath(t)

		// Save one entry that won't be removed
		if err := SaveForward(existingPort, 9000, "keep-space"); err != nil {
			return false
		}

		// Remove a port that doesn't exist
		if err := RemoveForward(removePort); err != nil {
			return false
		}

		// The existing entry should still be there
		entries, err := LoadForwards()
		if err != nil {
			return false
		}

		for _, e := range entries {
			if e.LocalPort == existingPort {
				return true
			}
		}
		return false
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Error(err)
	}
}

// --- Unit Tests (1.6) ---

func TestLoadForwards_NonExistentFile(t *testing.T) {
	setTestPath(t)

	entries, err := LoadForwards()
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty slice for missing file, got %d entries", len(entries))
	}
}

func TestRemoveForward_NonExistentFile(t *testing.T) {
	setTestPath(t)

	err := RemoveForward(8080)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
}

func TestIsPersistent_MissingFileOrAbsentEntry(t *testing.T) {
	setTestPath(t)

	// File doesn't exist
	if IsPersistent(8080) {
		t.Error("expected IsPersistent to return false when file is missing")
	}

	// File exists but entry is absent
	if err := SaveForward(9090, 3000, "other-space"); err != nil {
		t.Fatalf("SaveForward error: %v", err)
	}
	if IsPersistent(8080) {
		t.Error("expected IsPersistent to return false when entry is absent")
	}
}

func TestSaveForward_ReplacesExistingEntry(t *testing.T) {
	setTestPath(t)

	port := uint16(8080)

	if err := SaveForward(port, 6379, "redis-space"); err != nil {
		t.Fatalf("first SaveForward error: %v", err)
	}
	if err := SaveForward(port, 5432, "postgres-space"); err != nil {
		t.Fatalf("second SaveForward error: %v", err)
	}

	entries, err := LoadForwards()
	if err != nil {
		t.Fatalf("LoadForwards error: %v", err)
	}

	count := 0
	var found ForwardEntry
	for _, e := range entries {
		if e.LocalPort == port {
			count++
			found = e
		}
	}

	if count != 1 {
		t.Fatalf("expected exactly 1 entry for port %d, got %d", port, count)
	}
	if found.RemotePort != 5432 || found.Space != "postgres-space" {
		t.Errorf("expected updated entry {port=%d, space=postgres-space, remote=5432}, got %+v", port, found)
	}
}

func TestSaveAndLoad_MultipleEntries(t *testing.T) {
	setTestPath(t)

	entries := []ForwardEntry{
		{LocalPort: 8080, RemotePort: 6379, Space: "redis"},
		{LocalPort: 9090, RemotePort: 5432, Space: "postgres"},
		{LocalPort: 7070, RemotePort: 3000, Space: "api"},
	}

	for _, e := range entries {
		if err := SaveForward(e.LocalPort, e.RemotePort, e.Space); err != nil {
			t.Fatalf("SaveForward(%d) error: %v", e.LocalPort, err)
		}
	}

	loaded, err := LoadForwards()
	if err != nil {
		t.Fatalf("LoadForwards error: %v", err)
	}

	if len(loaded) != len(entries) {
		t.Fatalf("expected %d entries, got %d", len(entries), len(loaded))
	}

	// Build a map for easy lookup
	byPort := make(map[uint16]ForwardEntry)
	for _, e := range loaded {
		byPort[e.LocalPort] = e
	}

	for _, expected := range entries {
		got, ok := byPort[expected.LocalPort]
		if !ok {
			t.Errorf("missing entry for port %d", expected.LocalPort)
			continue
		}
		if got.RemotePort != expected.RemotePort || got.Space != expected.Space {
			t.Errorf("entry mismatch for port %d: got %+v, want %+v", expected.LocalPort, got, expected)
		}
	}
}

func TestIsPersistent_ReturnsTrue(t *testing.T) {
	setTestPath(t)

	if err := SaveForward(8080, 6379, "redis"); err != nil {
		t.Fatalf("SaveForward error: %v", err)
	}

	if !IsPersistent(8080) {
		t.Error("expected IsPersistent to return true for saved entry")
	}
}

func TestRemoveForward_IdempotentOnEmptyFile(t *testing.T) {
	setTestPath(t)

	// Call twice - both should return nil
	if err := RemoveForward(8080); err != nil {
		t.Fatalf("first RemoveForward error: %v", err)
	}
	if err := RemoveForward(8080); err != nil {
		t.Fatalf("second RemoveForward error: %v", err)
	}
}

// Fuzz-style test: save many random entries, verify no duplicates
func TestSaveForward_NoDuplicatesWithManyEntries(t *testing.T) {
	setTestPath(t)

	rng := rand.New(rand.NewSource(42))
	ports := []uint16{1000, 2000, 3000, 1000, 2000, 4000, 1000}

	for i, p := range ports {
		space := fmt.Sprintf("space-%d", i)
		if err := SaveForward(p, uint16(rng.Intn(60000)+1), space); err != nil {
			t.Fatalf("SaveForward(%d) error: %v", p, err)
		}
	}

	entries, err := LoadForwards()
	if err != nil {
		t.Fatalf("LoadForwards error: %v", err)
	}

	seen := make(map[uint16]bool)
	for _, e := range entries {
		if seen[e.LocalPort] {
			t.Errorf("duplicate entry for port %d", e.LocalPort)
		}
		seen[e.LocalPort] = true
	}
}
