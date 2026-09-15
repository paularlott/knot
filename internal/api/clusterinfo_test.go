package api

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

// A space with no node assignment (manual/Nomad platforms, or predating node
// pinning) must not be attributed to every node in the zone — each node
// counts only its own exact assignments.
func TestCountSpacesExactNodeAssignment(t *testing.T) {
	pinned := &model.Space{NodeId: "node-a", IsDeployed: true}
	unpinnedSameZone := &model.Space{NodeId: "", Zone: "zone1"}
	unpinnedRunning := &model.Space{NodeId: "", Zone: "zone1", IsDeployed: true}
	deleted := &model.Space{NodeId: "node-a", IsDeleted: true}
	other := &model.Space{NodeId: "node-b"}

	aAlloc, aRun := countSpaces([]*model.Space{pinned, unpinnedSameZone, unpinnedRunning, deleted, other}, "node-a")
	if aAlloc != 1 || aRun != 1 {
		t.Errorf("node-a: got %d/%d, want 1/1 (only its own pinned space)", aAlloc, aRun)
	}

	bAlloc, bRun := countSpaces([]*model.Space{pinned, unpinnedSameZone, unpinnedRunning, deleted, other}, "node-b")
	if bAlloc != 1 || bRun != 0 {
		t.Errorf("node-b: got %d/%d, want 1/0", bAlloc, bRun)
	}

	// A node with nothing pinned — including unpinned zone spaces — is 0/0.
	cAlloc, cRun := countSpaces([]*model.Space{pinned, unpinnedSameZone, unpinnedRunning, deleted, other}, "node-c")
	if cAlloc != 0 || cRun != 0 {
		t.Errorf("node-c: got %d/%d, want 0/0 — unpinned spaces count on no node", cAlloc, cRun)
	}
}
