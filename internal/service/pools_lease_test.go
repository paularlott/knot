package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func TestResolveLeaseDuration(t *testing.T) {
	cases := []struct {
		name        string
		poolMax     int
		requested   int
		wantSeconds int
		wantNever   bool
		wantErr     bool
	}{
		{name: "default uses pool max", poolMax: 600, requested: 0, wantSeconds: 600},
		{name: "default on unlimited pool is never", poolMax: -1, requested: 0, wantNever: true},
		{name: "explicit bounded", poolMax: 600, requested: 120, wantSeconds: 120},
		{name: "explicit over max rejected", poolMax: 600, requested: 601, wantErr: true},
		{name: "any duration on unlimited pool", poolMax: -1, requested: 999999, wantSeconds: 999999},
		{name: "never on unlimited pool", poolMax: -1, requested: -1, wantNever: true},
		{name: "never on limited pool rejected", poolMax: 600, requested: -1, wantErr: true},
		{name: "negative rejected", poolMax: 600, requested: -2, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seconds, never, err := resolveLeaseDuration(tc.poolMax, tc.requested)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got (%d, %v)", seconds, never)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if seconds != tc.wantSeconds || never != tc.wantNever {
				t.Fatalf("got (%d, never=%v), want (%d, never=%v)", seconds, never, tc.wantSeconds, tc.wantNever)
			}
		})
	}
}

func TestAcquireGrantsExclusiveLease(t *testing.T) {
	f := newReconcileFixture(t, "acquire")
	pool := f.pool("p", true, 1)
	pool.LeaseMaxTime = 3600
	m := f.member(t, pool, true, false, false)

	lease, err := f.svc.Acquire(context.Background(), pool, f.user, 60, 0)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if lease.SpaceId != m.Id {
		t.Fatalf("lease.SpaceId = %s, want %s", lease.SpaceId, m.Id)
	}
	if lease.State != "active" || lease.SpaceId == "" || lease.MaxExtensions != 0 {
		t.Fatalf("unexpected lease info: %#v", lease)
	}

	stored, err := database.GetInstance().GetSpace(m.Id)
	if err != nil {
		t.Fatalf("GetSpace: %v", err)
	}
	if stored.LeaseId == "" || stored.LeaseUserId != f.user.Id {
		t.Fatalf("lease fields not persisted: %#v", stored)
	}
	if stored.LeaseExpiresAt == nil || time.Until(*stored.LeaseExpiresAt) > 61*time.Second || time.Until(*stored.LeaseExpiresAt) < 55*time.Second {
		t.Fatalf("lease expiry not ~60s out: %v", stored.LeaseExpiresAt)
	}
	if !f.svc.IsLeased(m.Id) {
		t.Fatal("in-memory lease view should mark the member leased")
	}

	// The single member is now exclusively held: a second acquire is exhausted.
	if _, err := f.svc.Acquire(context.Background(), pool, f.user, 60, 0); !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("second Acquire error = %v, want ErrPoolExhausted", err)
	}
}

func TestAcquireNeverExpiringLease(t *testing.T) {
	f := newReconcileFixture(t, "acquire-never")
	pool := f.pool("p", true, 1)
	pool.LeaseMaxTime = -1
	pool.LeaseMaxExtensions = -1
	if err := database.GetInstance().SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	m := f.member(t, pool, true, false, false)

	lease, err := f.svc.Acquire(context.Background(), pool, f.user, 0, 0)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if lease.ExpiresAt != nil {
		t.Fatalf("lease.ExpiresAt = %v, want nil (never expires)", lease.ExpiresAt)
	}
	stored, _ := database.GetInstance().GetSpace(m.Id)
	if stored.LeaseExpiresAt != nil {
		t.Fatalf("persisted lease expiry = %v, want nil", stored.LeaseExpiresAt)
	}

	// The sweep must never reclaim it.
	f.svc.reconcileLeases([]*model.Space{stored})
	if !f.svc.IsLeased(m.Id) {
		t.Fatal("never-expiring lease must not be reclaimed by the sweep")
	}
}

func TestAcquireValidation(t *testing.T) {
	f := newReconcileFixture(t, "acquire-invalid")

	disabled := f.pool("disabled", true, 1)
	if _, err := f.svc.Acquire(context.Background(), disabled, f.user, 60, 0); !errors.Is(err, ErrPoolLeasesDisabled) {
		t.Fatalf("error = %v, want ErrPoolLeasesDisabled", err)
	}

	inactive := f.pool("inactive", false, 1)
	inactive.LeaseMaxTime = 3600
	if _, err := f.svc.Acquire(context.Background(), inactive, f.user, 60, 0); !errors.Is(err, ErrPoolNotActive) {
		t.Fatalf("error = %v, want ErrPoolNotActive", err)
	}

	overMax := f.pool("overmax", true, 1)
	overMax.LeaseMaxTime = 100
	if _, err := f.svc.Acquire(context.Background(), overMax, f.user, 200, 0); !errors.Is(err, ErrLeaseDuration) {
		t.Fatalf("error = %v, want ErrLeaseDuration", err)
	}
}

func TestExtendHonoursExtensionCap(t *testing.T) {
	f := newReconcileFixture(t, "extend")
	pool := f.pool("p", true, 1)
	pool.LeaseMaxTime = 3600
	pool.LeaseMaxExtensions = 1
	if err := database.GetInstance().SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	f.member(t, pool, true, false, false)

	lease, err := f.svc.Acquire(context.Background(), pool, f.user, 60, 0)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	extended, err := f.svc.Extend(f.user, lease.SpaceName, 60)
	if err != nil {
		t.Fatalf("Extend: %v", err)
	}
	if extended.ExtensionsUsed != 1 {
		t.Fatalf("ExtensionsUsed = %d, want 1", extended.ExtensionsUsed)
	}

	if _, err := f.svc.Extend(f.user, lease.SpaceName, 60); !errors.Is(err, ErrLeaseExtendLimit) {
		t.Fatalf("second Extend error = %v, want ErrLeaseExtendLimit", err)
	}
}

func TestExtendUnlimitedAndDisabled(t *testing.T) {
	f := newReconcileFixture(t, "extend-unlimited")

	unlimited := f.pool("unlimited", true, 1)
	unlimited.LeaseMaxTime = 3600
	unlimited.LeaseMaxExtensions = -1
	if err := database.GetInstance().SavePoolDefinition(unlimited, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	f.member(t, unlimited, true, false, false)
	lease, err := f.svc.Acquire(context.Background(), unlimited, f.user, 60, 0)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := f.svc.Extend(f.user, lease.SpaceName, 60); err != nil {
			t.Fatalf("Extend %d on unlimited: %v", i+1, err)
		}
	}

	disabled := f.pool("nodisable", true, 1)
	disabled.LeaseMaxTime = 3600
	disabled.LeaseMaxExtensions = 0
	if err := database.GetInstance().SavePoolDefinition(disabled, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	f.member(t, disabled, true, false, false)
	lease2, err := f.svc.Acquire(context.Background(), disabled, f.user, 60, 0)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := f.svc.Extend(f.user, lease2.SpaceName, 60); !errors.Is(err, ErrLeaseExtendLimit) {
		t.Fatalf("Extend on forbidden error = %v, want ErrLeaseExtendLimit", err)
	}
}

// Disabling leases on a pool after grants must let held leases end naturally
// (expiry/release) but refuse renewals.
func TestExtendRejectedWhenLeasesDisabled(t *testing.T) {
	f := newReconcileFixture(t, "extend-disabled")
	pool := f.pool("p", true, 1)
	pool.LeaseMaxTime = 3600
	pool.LeaseMaxExtensions = 5
	if err := database.GetInstance().SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	f.member(t, pool, true, false, false)

	lease, err := f.svc.Acquire(context.Background(), pool, f.user, 3600, 0)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// The pool config changes to disabled (persisted, as the update API
	// would — the space-keyed Extend loads the pool from the database).
	pool.LeaseMaxTime = 0
	pool.LeaseMaxExtensions = 0
	if err := database.GetInstance().SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	if _, err := f.svc.Extend(f.user, lease.SpaceName, 60); !errors.Is(err, ErrPoolLeasesDisabled) {
		t.Fatalf("Extend on disabled pool error = %v, want ErrPoolLeasesDisabled", err)
	}

	// Release still works — the holder can hand the member back.
	if _, err := f.svc.Release(f.user, lease.SpaceName); err != nil {
		t.Fatalf("Release on disabled pool: %v", err)
	}
}

// ReleaseAndDestroy ends the lease, marks the member for deletion, and
// creates a fresh replacement immediately.
func TestReleaseAndDestroy(t *testing.T) {
	f := newReconcileFixture(t, "destroy")
	pool := f.pool("p", true, 1)
	pool.LeaseMaxTime = -1
	pool.LeaseMaxExtensions = -1
	if err := database.GetInstance().SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	m := f.member(t, pool, true, false, false)
	f.tmpl.Active = true
	if err := database.GetInstance().SaveTemplate(f.tmpl, nil); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	lease, err := f.svc.Acquire(context.Background(), pool, f.user, 0, 0)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	info, err := f.svc.ReleaseAndDestroy(f.user, lease.SpaceName)
	if err != nil {
		t.Fatalf("ReleaseAndDestroy: %v", err)
	}
	if info.State != "destroying" {
		t.Fatalf("state = %s, want destroying", info.State)
	}

	// The member is deleting and no longer holds a lease.
	stored, _ := database.GetInstance().GetSpace(m.Id)
	if !stored.IsDeleting {
		t.Fatal("member should be marked deleting")
	}
	if stored.LeaseId != "" || f.svc.IsLeased(m.Id) {
		t.Fatal("lease should be cleared from record and routing view")
	}
	if f.fc.deletedCount(m.Id) != 1 {
		t.Fatalf("container delete count = %d, want 1", f.fc.deletedCount(m.Id))
	}

	// A replacement member was created immediately.
	spaces, _ := database.GetInstance().GetSpaces()
	replacements := 0
	for _, sp := range spaces {
		if sp.PoolId == pool.Id && sp.Id != m.Id && !sp.IsDeleted {
			replacements++
		}
	}
	if replacements != 1 {
		t.Fatalf("replacement members = %d, want 1", replacements)
	}
	if f.fc.startedTotal() < 1 {
		t.Fatal("replacement should have been started")
	}
}

func TestReleaseDrainsThenReclaims(t *testing.T) {
	f := newReconcileFixture(t, "release")
	pool := f.pool("p", true, 1)
	pool.LeaseMaxTime = 3600
	if err := database.GetInstance().SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	m := f.member(t, pool, true, false, false)

	lease, err := f.svc.Acquire(context.Background(), pool, f.user, 3600, 0)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	released, err := f.svc.Release(f.user, lease.SpaceName)
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if released.State != "draining" {
		t.Fatalf("released lease state = %s, want draining", released.State)
	}
	// Still excluded from routing until the sweep reclaims it.
	if !f.svc.IsLeased(m.Id) {
		t.Fatal("released lease must stay excluded until reclaimed")
	}

	// The sweep (idle member) reclaims it.
	stored, _ := database.GetInstance().GetSpace(m.Id)
	f.svc.reconcileLeases([]*model.Space{stored})
	if f.svc.IsLeased(m.Id) {
		t.Fatal("sweep should have reclaimed the ended lease")
	}
	stored, _ = database.GetInstance().GetSpace(m.Id)
	if stored.LeaseId != "" || stored.LeaseExpiresAt != nil || stored.LeaseExtensions != 0 {
		t.Fatalf("lease fields not cleared: %#v", stored)
	}
}

func TestSweepWaitsForInFlightWork(t *testing.T) {
	f := newReconcileFixture(t, "drain")
	pool := f.pool("p", true, 1)
	pool.LeaseMaxTime = 1 // second-granularity expiry
	m := f.member(t, pool, true, false, false)

	if _, err := f.svc.Acquire(context.Background(), pool, f.user, 1, 0); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Force the expiry into the past.
	stored, _ := database.GetInstance().GetSpace(m.Id)
	past := time.Now().Add(-time.Minute)
	stored.LeaseExpiresAt = &past
	if err := database.GetInstance().SaveSpace(stored, []string{"LeaseExpiresAt"}); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}
	f.svc.SyncSpaceLease(stored)

	// The agent reports work in flight: the lease must be held.
	prev := poolSessionProvider
	SetPoolSessionProvider(func(spaceID string) *PoolSessionState {
		return &PoolSessionState{ActiveMethodCalls: 2}
	})
	f.svc.reconcileLeases([]*model.Space{stored})
	if !f.svc.IsLeased(m.Id) {
		t.Fatal("sweep reclaimed a lease with in-flight work")
	}

	// Work drained: the member returns to the pool.
	SetPoolSessionProvider(func(spaceID string) *PoolSessionState {
		return &PoolSessionState{ActiveMethodCalls: 0}
	})
	f.svc.reconcileLeases([]*model.Space{stored})
	if f.svc.IsLeased(m.Id) {
		t.Fatal("sweep did not reclaim the drained lease")
	}
	SetPoolSessionProvider(prev)
}

// The sweep loads spaces at SweepOnce start; if a lease is extended after
// that load, the reclaim must notice on its fresh read under the lock and
// leave the (now unexpired) lease alone instead of clobbering it.
func TestReclaimLeaseRevalidatesUnderLock(t *testing.T) {
	f := newReconcileFixture(t, "reclaim-race")
	pool := f.pool("p", true, 1)
	pool.LeaseMaxTime = 3600
	pool.LeaseMaxExtensions = 5
	if err := database.GetInstance().SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	m := f.member(t, pool, true, false, false)

	lease, err := f.svc.Acquire(context.Background(), pool, f.user, 60, 0)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// The sweep's stale copy: loaded, then the lease expires...
	past := time.Now().Add(-time.Minute)
	stale, _ := database.GetInstance().GetSpace(m.Id)
	stale.LeaseExpiresAt = &past
	if err := database.GetInstance().SaveSpace(stale, []string{"LeaseExpiresAt"}); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}
	stale, _ = database.GetInstance().GetSpace(m.Id)

	// ...but before the sweep acts, the holder extends the lease.
	if _, err := f.svc.Extend(f.user, lease.SpaceName, 3600); err != nil {
		t.Fatalf("Extend: %v", err)
	}

	// The reclaim working from the stale copy must not clear the lease.
	f.svc.reclaimLease(stale)
	if !f.svc.IsLeased(m.Id) {
		t.Fatal("reclaim clobbered a lease that was extended after the sweep's copy was loaded")
	}
	stored, _ := database.GetInstance().GetSpace(m.Id)
	if stored.LeaseId == "" || stored.LeaseExpiresAt == nil {
		t.Fatalf("extended lease was lost: %#v", stored)
	}

	// Once the extension genuinely lapses, the same path reclaims it.
	lapsed := time.Now().Add(-time.Minute)
	stored.LeaseExpiresAt = &lapsed
	if err := database.GetInstance().SaveSpace(stored, []string{"LeaseExpiresAt"}); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}
	f.svc.reclaimLease(stale)
	if f.svc.IsLeased(m.Id) {
		t.Fatal("sweep did not reclaim a genuinely lapsed lease")
	}
}

func TestHandleExcessSkipsLeasedMembers(t *testing.T) {
	f := newReconcileFixture(t, "excess-leased")
	pool := f.pool("p", true, 1)
	leased := f.member(t, pool, true, false, false)
	free := f.member(t, pool, true, false, false)

	leased.LeaseId = "lease-1"
	leased.LeaseUserId = f.user.Id
	expires := time.Now().Add(time.Hour)
	leased.LeaseExpiresAt = &expires
	if err := database.GetInstance().SaveSpace(leased, []string{"LeaseId", "LeaseUserId", "LeaseExpiresAt"}); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}
	f.svc.SyncSpaceLease(leased)

	f.svc.handleExcess(pool, []*model.Space{leased, free})

	if f.svc.isDrained(leased.Id) || f.fc.stoppedCount(leased.Id) != 0 {
		t.Fatal("leased member must not be shrunk")
	}
	if !f.svc.isDrained(free.Id) {
		t.Fatal("free member should be the shrink candidate")
	}
}

func TestStopRejectsActiveLease(t *testing.T) {
	f := newReconcileFixture(t, "stop-leased")
	pool := f.pool("p", true, 1)
	pool.LeaseMaxTime = 3600
	m := f.member(t, pool, true, false, false)

	if _, err := f.svc.Acquire(context.Background(), pool, f.user, 3600, 0); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := f.svc.Stop(pool, f.user); err == nil {
		t.Fatal("Stop should reject a pool with an active lease")
	}

	// Persist the pool so the post-release stop path can proceed.
	if err := database.GetInstance().SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	releaseLease(t, f, pool, m)
	if err := f.svc.Stop(pool, f.user); err != nil {
		t.Fatalf("Stop after release: %v", err)
	}
}

func releaseLease(t *testing.T, f *reconcileFixture, pool *model.PoolDefinition, m *model.Space) {
	t.Helper()
	stored, err := database.GetInstance().GetSpace(m.Id)
	if err != nil {
		t.Fatalf("GetSpace: %v", err)
	}
	_, err = f.svc.Release(f.user, stored.Name)
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	stored, _ = database.GetInstance().GetSpace(m.Id)
	f.svc.reconcileLeases([]*model.Space{stored})
}
