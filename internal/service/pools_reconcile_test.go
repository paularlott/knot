package service

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// freshPoolService returns an isolated PoolService so drain/pending state does
// not leak between tests (the production singleton is process-wide).
func freshPoolService() *PoolService {
	return &PoolService{
		pendingDeletions: make(map[string]time.Time),
		drained:          make(map[string]bool),
		rrCounters:       make(map[string]int),
	}
}

type reconcileFixture struct {
	svc  *PoolService
	fc   *fakeContainer
	ft   *fakeTransport
	user *model.User
	tmpl *model.Template
}

func newReconcileFixture(t *testing.T, name string) *reconcileFixture {
	t.Helper()
	restorePoolDeps(t)
	setServerZone(t, "z1")
	ft := &fakeTransport{leader: true}
	SetTransport(ft)
	fc := &fakeContainer{}
	SetContainerService(fc)

	tmpl := newPoolTestTemplate(t, "recon-tmpl-"+name)
	user := newPoolTestUser("recon-user-" + name)
	user.Username = "recon-user-" + name
	user.Email = "recon-user-" + name + "@test.local"
	if err := database.GetInstance().SaveUser(user, nil); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	return &reconcileFixture{svc: freshPoolService(), fc: fc, ft: ft, user: user, tmpl: tmpl}
}

func (f *reconcileFixture) pool(name string, active bool, desired int) *model.PoolDefinition {
	p := model.NewPoolDefinition(name, f.tmpl.Id, "", desired, f.user.Id)
	p.Active = active
	p.Zone = "z1"
	p.CreatedUserId = f.user.Id
	return p
}

func (f *reconcileFixture) member(t *testing.T, pool *model.PoolDefinition, deployed, pending, deleting bool) *model.Space {
	t.Helper()
	sp := model.NewSpace("m-"+uuid.NewString(), "", f.user.Id, f.tmpl.Id, "bash", &[]model.AltNameEntry{}, "z1", "", nil)
	sp.PoolId = pool.Id
	sp.IsDeployed = deployed
	sp.IsPending = pending
	sp.IsDeleting = deleting
	if err := database.GetInstance().SaveSpace(sp, nil); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}
	return sp
}

// --- reconcile: transitions in progress ------------------------------------

func TestReconcileWaitsWhilePending(t *testing.T) {
	f := newReconcileFixture(t, "pending")
	pool := f.pool("p", true, 1)
	m := f.member(t, pool, true, true, false) // pending

	f.svc.reconcile(pool, []*model.Space{m})

	if f.fc.stoppedCount(m.Id)+f.fc.startedCount(m.Id)+f.fc.deletedCount(m.Id) != 0 {
		t.Fatal("reconcile must take no action while a member is pending")
	}
}

func TestReconcileRetriesDeletingMember(t *testing.T) {
	f := newReconcileFixture(t, "deleting")
	pool := f.pool("p", true, 1)
	m := f.member(t, pool, false, false, true) // deleting

	f.svc.reconcile(pool, []*model.Space{m})

	if f.fc.deletedCount(m.Id) != 1 {
		t.Fatalf("reconcile should retry DeleteSpace for a deleting member, got %d", f.fc.deletedCount(m.Id))
	}
}

// --- reconcile: scale up (create new members) ------------------------------

// With no members and an active pool, reconcile creates spaces up to
// DesiredCount and starts each one. Exercises createPoolSpace end-to-end
// (the real SpaceService.CreateSpace against the test DB, then StartSpace).
func TestReconcileCreatesUpToDesiredCount(t *testing.T) {
	f := newReconcileFixture(t, "create")
	// CreateSpace requires an active template.
	f.tmpl.Active = true
	if err := database.GetInstance().SaveTemplate(f.tmpl, nil); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	pool := f.pool("p", true, 2)

	f.svc.reconcile(pool, nil)

	spaces, err := database.GetInstance().GetSpaces()
	if err != nil {
		t.Fatalf("GetSpaces: %v", err)
	}
	created := 0
	for _, sp := range spaces {
		if sp.PoolId == pool.Id {
			created++
		}
	}
	if created != 2 {
		t.Fatalf("created %d pool members, want 2", created)
	}
	if f.fc.startedTotal() != 2 {
		t.Fatalf("StartSpace called %d times, want 2", f.fc.startedTotal())
	}
}

// --- reconcile: scale up (start stopped member) ----------------------------

func TestReconcileStartsStoppedMember(t *testing.T) {
	f := newReconcileFixture(t, "start")
	pool := f.pool("p", true, 1)
	m := f.member(t, pool, false, false, false) // stopped

	f.svc.reconcile(pool, []*model.Space{m})

	if f.fc.startedCount(m.Id) != 1 {
		t.Fatalf("reconcile should start a stopped member up to DesiredCount, got %d", f.fc.startedCount(m.Id))
	}
}

// --- drain ↔ stop two-pass for running excess ------------------------------

// A running excess member is drained first, stopped on the next sweep, then the
// reconciler waits while it is pending (the StopSpace async contract).
func TestReconcileDrainThenStopThenWaitForRunningExcess(t *testing.T) {
	f := newReconcileFixture(t, "drainstop")
	pool := f.pool("p", true, 0) // everything is excess
	m := f.member(t, pool, true, false, false)

	// Pass 1: drain, no stop yet.
	f.svc.reconcile(pool, []*model.Space{m})
	if !f.svc.isDrained(m.Id) {
		t.Fatal("pass 1 should drain the running excess member")
	}
	if f.fc.stoppedCount(m.Id) != 0 {
		t.Fatal("pass 1 must not stop the member yet")
	}

	// Pass 2: already drained → stop. The fake StopSpace marks it pending.
	f.svc.reconcile(pool, []*model.Space{m})
	if f.fc.stoppedCount(m.Id) != 1 {
		t.Fatalf("pass 2 should stop the drained excess member, got %d", f.fc.stoppedCount(m.Id))
	}
	if !m.IsPending {
		t.Fatal("StopSpace must mark the member pending synchronously")
	}

	// Pass 3: member is pending → reconciler waits, no second stop.
	f.svc.reconcile(pool, []*model.Space{m})
	if f.fc.stoppedCount(m.Id) != 1 {
		t.Fatalf("pass 3 must not stop again while pending, got %d", f.fc.stoppedCount(m.Id))
	}
}

// --- grace-period (two-pass) deletion for stopped excess -------------------

func TestReconcileGracePeriodDeletesStoppedExcess(t *testing.T) {
	f := newReconcileFixture(t, "grace")
	pool := f.pool("p", true, 0)
	m := f.member(t, pool, false, false, false) // stopped excess

	// Pass 1: mark pending for deletion, do not delete yet.
	f.svc.reconcile(pool, []*model.Space{m})
	if !f.svc.isPending(m.Id) {
		t.Fatal("pass 1 should mark the stopped excess member pending for deletion")
	}
	if f.fc.deletedCount(m.Id) != 0 {
		t.Fatal("pass 1 must not delete during the grace period")
	}

	// Pass 2: grace elapsed (already pending) → delete.
	f.svc.reconcile(pool, []*model.Space{m})
	if f.fc.deletedCount(m.Id) != 1 {
		t.Fatalf("pass 2 should delete the stopped excess member, got %d", f.fc.deletedCount(m.Id))
	}
}

// Scaling back up before the grace period elapses reuses the member instead of
// deleting it: a keeper is undrained and its pending-deletion mark cleared.
func TestReconcileUndrainsKeeper(t *testing.T) {
	f := newReconcileFixture(t, "undrain")
	pool := f.pool("p", true, 1)
	m := f.member(t, pool, true, false, false)

	// Pre-drain and pre-mark as if a previous scale-down had started.
	f.svc.drain(m.Id)
	f.svc.markPending(m.Id)
	f.ft.undrained = nil // ignore the drain's gossip for the assertion below

	f.svc.reconcile(pool, []*model.Space{m})

	if f.svc.isDrained(m.Id) {
		t.Fatal("a kept member must be undrained")
	}
	if f.svc.isPending(m.Id) {
		t.Fatal("a kept member's pending-deletion mark must be cleared")
	}
	if len(f.ft.undrained) == 0 || f.ft.undrained[len(f.ft.undrained)-1] != m.Id {
		t.Fatal("undrain should be gossiped for the kept member")
	}
}

// --- handleExcess directly: keepers vs excess ------------------------------

func TestHandleExcessKeepsFirstDrainsRest(t *testing.T) {
	f := newReconcileFixture(t, "excess")
	pool := f.pool("p", true, 1)
	keep := f.member(t, pool, true, false, false)
	drop := f.member(t, pool, true, false, false)
	f.svc.drain(keep.Id) // keeper starts drained; handleExcess should undrain it

	f.svc.handleExcess([]*model.Space{keep, drop}, 1)

	if f.svc.isDrained(keep.Id) {
		t.Fatal("keeper (within DesiredCount) must be undrained")
	}
	if !f.svc.isDrained(drop.Id) {
		t.Fatal("excess running member must be drained")
	}
}

// --- stopped pool ----------------------------------------------------------

func TestReconcileStoppedPoolStopsDeployed(t *testing.T) {
	f := newReconcileFixture(t, "stoppedpool")
	pool := f.pool("p", false, 2) // inactive
	m := f.member(t, pool, true, false, false)

	f.svc.reconcileStoppedPool(pool, []*model.Space{m})

	if f.fc.stoppedCount(m.Id) != 1 {
		t.Fatalf("stopped pool should stop a deployed member, got %d", f.fc.stoppedCount(m.Id))
	}
	if !f.svc.isDrained(m.Id) {
		t.Fatal("stopped pool should drain before stopping")
	}
}

func TestReconcileStoppedPoolDeletesExcessStopped(t *testing.T) {
	f := newReconcileFixture(t, "stoppedexcess")
	pool := f.pool("p", false, 1)
	keep := f.member(t, pool, false, false, false)
	excess := f.member(t, pool, false, false, false)

	f.svc.reconcileStoppedPool(pool, []*model.Space{keep, excess})

	if f.fc.deletedCount(excess.Id) != 1 {
		t.Fatalf("stopped pool should delete the excess stopped member, got %d", f.fc.deletedCount(excess.Id))
	}
	if f.fc.deletedCount(keep.Id) != 0 {
		t.Fatal("stopped pool must keep DesiredCount stopped members")
	}
}

// --- deleted pool ----------------------------------------------------------

func TestReconcileDeletedPoolDrainsAndDeletes(t *testing.T) {
	f := newReconcileFixture(t, "deletedpool")
	pool := f.pool("p", true, 1)
	pool.IsDeleted = true
	m := f.member(t, pool, false, false, false)

	f.svc.reconcileDeletedPool([]*model.Space{m})

	if !f.svc.isDrained(m.Id) {
		t.Fatal("deleted pool should drain its members")
	}
	if f.fc.deletedCount(m.Id) != 1 {
		t.Fatalf("deleted pool should delete its members, got %d", f.fc.deletedCount(m.Id))
	}
}
