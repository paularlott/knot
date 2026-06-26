package service

import (
	"sync"
	"testing"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// fakeContainer records the lifecycle calls the pool reconciler makes so zone
// ownership can be asserted without a real container backend.
type fakeContainer struct {
	mu      sync.Mutex
	deleted []string
	stopped []string
	started []string
}

func (c *fakeContainer) CreateVolume(*model.Volume) error { return nil }
func (c *fakeContainer) DeleteVolume(*model.Volume) error { return nil }
func (c *fakeContainer) StartSpace(space *model.Space, _ *model.Template, _ *model.User) error {
	c.mu.Lock()
	c.started = append(c.started, space.Id)
	c.mu.Unlock()
	return nil
}
func (c *fakeContainer) StopSpace(space *model.Space) error {
	c.mu.Lock()
	c.stopped = append(c.stopped, space.Id)
	c.mu.Unlock()
	// Mirror the real StopSpace, which marks the space pending synchronously
	// before tearing down. The pool reconciler depends on this to avoid acting
	// on the same space twice while it is stopping.
	space.IsPending = true
	space.UpdatedAt = hlc.Now()
	_ = database.GetInstance().SaveSpace(space, []string{"IsPending", "UpdatedAt"})
	return nil
}

func (c *fakeContainer) stoppedCount(id string) int { return c.countIn(c.stopped, id) }
func (c *fakeContainer) startedCount(id string) int { return c.countIn(c.started, id) }
func (c *fakeContainer) startedTotal() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.started)
}
func (c *fakeContainer) countIn(list []string, id string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, x := range list {
		if x == id {
			n++
		}
	}
	return n
}
func (c *fakeContainer) RestartSpace(*model.Space) error { return nil }
func (c *fakeContainer) DeleteSpace(space *model.Space) {
	c.mu.Lock()
	c.deleted = append(c.deleted, space.Id)
	c.mu.Unlock()
}
func (c *fakeContainer) CleanupOnBoot() {}

func (c *fakeContainer) deletedCount(id string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, d := range c.deleted {
		if d == id {
			n++
		}
	}
	return n
}

func setServerZone(t *testing.T, zone string) {
	t.Helper()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
		Zone:     zone,
	})
	model.SetRoleCache(nil)
}

func restorePoolDeps(t *testing.T) {
	t.Helper()
	prevT := GetTransport()
	prevC := containerService
	t.Cleanup(func() {
		SetTransport(prevT)
		SetContainerService(prevC)
	})
}

// Create stamps the pool with the server's current zone so each zone's leader
// only manages the pools created in that zone.
func TestPoolCreateStampsServerZone(t *testing.T) {
	restorePoolDeps(t)
	setServerZone(t, "zone-stamp")
	template := newPoolTestTemplate(t, "pool-zone-stamp-tmpl")
	user := newPoolTestUser("pool-zone-stamp-user")

	pool := model.NewPoolDefinition("pool-zone-stamp", template.Id, "", 1, user.Id)
	pool.Active = false
	if err := GetPoolService().Create(pool, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if pool.Zone != "zone-stamp" {
		t.Fatalf("pool.Zone = %q, want %q", pool.Zone, "zone-stamp")
	}
	stored, err := database.GetInstance().GetPoolDefinition(pool.Id)
	if err != nil {
		t.Fatalf("GetPoolDefinition: %v", err)
	}
	if stored.Zone != "zone-stamp" {
		t.Fatalf("stored pool.Zone = %q, want %q", stored.Zone, "zone-stamp")
	}
}

// SweepOnce reconciles a pool only on the leader of the pool's owning zone. A
// server in a different zone must leave the pool's members untouched.
func TestSweepOnceManagesOnlyOwningZone(t *testing.T) {
	restorePoolDeps(t)
	ft := &fakeTransport{leader: true}
	SetTransport(ft)
	SetContainerService(&fakeContainer{})

	// Build an active pool owned by zone "z1" with one excess (deployed) member,
	// so reconcile wants to drain the member. Save directly to control the zone.
	setServerZone(t, "z1")
	template := newPoolTestTemplate(t, "pool-zone-sweep-tmpl")
	user := newPoolTestUser("pool-zone-sweep-user")
	user.Username = "pool-zone-sweep-user"
	user.Email = "pool-zone-sweep-user@test.local"
	if err := database.GetInstance().SaveUser(user, nil); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	pool := model.NewPoolDefinition("pool-zone-sweep", template.Id, "", 0, user.Id)
	pool.Active = true
	pool.Zone = "z1"
	if err := database.GetInstance().SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}

	member := model.NewSpace("pool-zone-sweep-member", "", user.Id, template.Id, "bash", &[]model.AltNameEntry{}, "z1", "", nil)
	member.PoolId = pool.Id
	member.IsDeployed = true
	if err := database.GetInstance().SaveSpace(member, nil); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}

	svc := GetPoolService()

	// A server in a foreign zone must not touch the pool.
	setServerZone(t, "z2")
	if err := svc.SweepOnce(); err != nil {
		t.Fatalf("SweepOnce (foreign zone): %v", err)
	}
	if svc.isDrained(member.Id) {
		t.Fatal("foreign-zone server drained a pool member it does not own")
	}

	// The owning zone's leader reconciles it (drains the excess member).
	setServerZone(t, "z1")
	if err := svc.SweepOnce(); err != nil {
		t.Fatalf("SweepOnce (owning zone): %v", err)
	}
	if !svc.isDrained(member.Id) {
		t.Fatal("owning-zone leader did not reconcile its own pool member")
	}
}

// ReapOrphans removes orphaned pool spaces only in the local zone; spaces in
// other zones are left for their own zone's server to reap.
func TestReapOrphansOnlyReapsLocalZone(t *testing.T) {
	restorePoolDeps(t)
	ft := &fakeTransport{leader: true}
	SetTransport(ft)
	fc := &fakeContainer{}
	SetContainerService(fc)

	setServerZone(t, "z1")
	template := newPoolTestTemplate(t, "pool-zone-reap-tmpl")
	user := newPoolTestUser("pool-zone-reap-user")
	user.Username = "pool-zone-reap-user"
	user.Email = "pool-zone-reap-user@test.local"
	if err := database.GetInstance().SaveUser(user, nil); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	// Orphan space (its pool no longer exists) belonging to zone z1.
	orphan := model.NewSpace("pool-zone-reap-orphan", "", user.Id, template.Id, "bash", &[]model.AltNameEntry{}, "z1", "", nil)
	orphan.PoolId = "ghost-pool-id"
	orphan.IsDeployed = true
	if err := database.GetInstance().SaveSpace(orphan, nil); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}

	svc := GetPoolService()

	// A foreign-zone server must skip the orphan.
	setServerZone(t, "z2")
	if err := svc.ReapOrphans(); err != nil {
		t.Fatalf("ReapOrphans (foreign zone): %v", err)
	}
	if fc.deletedCount(orphan.Id) != 0 {
		t.Fatal("foreign-zone server reaped an orphan it does not own")
	}
	if reloaded, err := database.GetInstance().GetSpace(orphan.Id); err != nil {
		t.Fatalf("GetSpace: %v", err)
	} else if reloaded.IsDeleting {
		t.Fatal("foreign-zone server marked a foreign orphan for deletion")
	}

	// The owning zone reaps it.
	setServerZone(t, "z1")
	if err := svc.ReapOrphans(); err != nil {
		t.Fatalf("ReapOrphans (owning zone): %v", err)
	}
	if fc.deletedCount(orphan.Id) != 1 {
		t.Fatalf("owning-zone server DeleteSpace count = %d, want 1", fc.deletedCount(orphan.Id))
	}
}
