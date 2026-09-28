package methods

import (
	"errors"
	"testing"
)

// A leased space's methods disappear from shared routing for everyone,
// with the distinct ErrMethodLeased error when no other provider exists.
func TestPickSkipsLeasedEntries(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	_, other := roleWithMethods()

	free := testSpace("space-1", "free", owner.Id)
	leased := testSpace("space-2", "leased", owner.Id)
	if err := registry.Register(free, owner, testRegistration("test", ScopeShared)); err != nil {
		t.Fatalf("Register(free): %v", err)
	}
	if err := registry.Register(leased, owner, testRegistration("test", ScopeShared)); err != nil {
		t.Fatalf("Register(leased): %v", err)
	}
	registry.SetLeaseChecker(func(spaceID string) bool {
		return spaceID == leased.Id
	})

	// Owner routing: the free provider is still picked, never the leased one.
	for i := 0; i < 4; i++ {
		entry, _, err := registry.Pick("test", owner)
		if err != nil {
			t.Fatalf("Pick() error = %v", err)
		}
		if entry.SpaceID == leased.Id {
			t.Fatal("Pick() routed to a leased space")
		}
		registry.Done(entry)
	}

	// Shared (non-owner) routing sees the free provider under the namespace.
	entry, _, err := registry.Pick("user.paul.test", other)
	if err != nil {
		t.Fatalf("Pick(namespaced) error = %v", err)
	}
	if entry.SpaceID == leased.Id {
		t.Fatal("Pick(namespaced) routed to a leased space")
	}
	registry.Done(entry)
}

// When every visible provider is leased, Pick reports ErrMethodLeased
// rather than not-found or draining.
func TestPickAllLeasedReturnsLeasedError(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	leased := testSpace("space-1", "only", owner.Id)
	if err := registry.Register(leased, owner, testRegistration("test", ScopeShared)); err != nil {
		t.Fatalf("Register(): %v", err)
	}
	registry.SetLeaseChecker(func(spaceID string) bool { return true })

	if _, _, err := registry.Pick("test", owner); !errors.Is(err, ErrMethodLeased) {
		t.Fatalf("Pick() error = %v, want ErrMethodLeased", err)
	}
}

// List excludes leased providers from the provider count.
func TestListExcludesLeasedProviders(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	free := testSpace("space-1", "free", owner.Id)
	leased := testSpace("space-2", "leased", owner.Id)
	if err := registry.Register(free, owner, testRegistration("test", ScopeShared)); err != nil {
		t.Fatalf("Register(free): %v", err)
	}
	if err := registry.Register(leased, owner, testRegistration("test", ScopeShared)); err != nil {
		t.Fatalf("Register(leased): %v", err)
	}
	registry.SetLeaseChecker(func(spaceID string) bool {
		return spaceID == leased.Id
	})

	list := registry.List(owner)
	if len(list) != 1 {
		t.Fatalf("List() = %d methods, want 1", len(list))
	}
	if list[0].ProviderCount != 1 {
		t.Fatalf("ProviderCount = %d, want 1 (leased provider excluded)", list[0].ProviderCount)
	}
}

// PickForSpace routes to the leased space directly, bypassing the lease
// exclusion — this is how a lease holder reaches its member.
func TestPickForSpaceBypassesLease(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	leased := testSpace("space-1", "only", owner.Id)
	if err := registry.Register(leased, owner, testRegistrationWithLocalName("test", "do_test", ScopeShared)); err != nil {
		t.Fatalf("Register(): %v", err)
	}
	registry.SetLeaseChecker(func(spaceID string) bool { return true })

	entry, localName, err := registry.PickForSpace("test", leased.Id, owner)
	if err != nil {
		t.Fatalf("PickForSpace() error = %v", err)
	}
	if entry.SpaceID != leased.Id || localName != "do_test" {
		t.Fatalf("PickForSpace() = %s/%s, want %s/do_test", entry.SpaceID, localName, leased.Id)
	}
	registry.Done(entry)
}

// PickForSpace also bypasses drain, and honours visibility for the
// namespaced non-owner form.
func TestPickForSpaceNamespacedAndErrors(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	_, other := roleWithMethods()
	noPerm := testUser("user-3", "bob", nil)
	space := testSpace("space-1", "only", owner.Id)
	if err := registry.Register(space, owner, testRegistration("open.page", ScopeShared)); err != nil {
		t.Fatalf("Register(): %v", err)
	}
	registry.Drain(space.Id)

	if _, _, err := registry.PickForSpace("user.paul.open.page", space.Id, other); err != nil {
		t.Fatalf("PickForSpace(namespaced) error = %v", err)
	}

	// Not visible to the caller.
	if _, _, err := registry.PickForSpace("open.page", space.Id, noPerm); !errors.Is(err, ErrPermission) {
		t.Fatalf("PickForSpace(no permission) error = %v, want ErrPermission", err)
	}

	// Method exists on the space but under a different name.
	if _, _, err := registry.PickForSpace("other", space.Id, owner); !errors.Is(err, ErrMethodNotFound) {
		t.Fatalf("PickForSpace(unknown) error = %v, want ErrMethodNotFound", err)
	}
}

// A space with no entries returns ErrMethodNotFound from PickForSpace.
func TestPickForSpaceUnknownSpace(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	if _, _, err := registry.PickForSpace("test", "ghost", owner); !errors.Is(err, ErrMethodNotFound) {
		t.Fatalf("PickForSpace(unknown space) error = %v, want ErrMethodNotFound", err)
	}
}
