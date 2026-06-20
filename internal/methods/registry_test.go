package methods

import (
	"errors"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func testSpace(id, name, userID string) *model.Space {
	return &model.Space{Id: id, Name: name, UserId: userID}
}

func testUser(id, username string, groups []string) *model.User {
	return &model.User{Id: id, Username: username, Groups: groups}
}

// PermissionUseMethods lives on model.Role as a uint16 permission bit; we use
// the raw constant via model to avoid importing the role.go iota block here.
const PermissionUseMethods = model.PermissionUseMethods

func testRegistration(name string, scope string) *Registration {
	return &Registration{
		Server: ServerConfig{Type: ServerTypeStdio, Command: "./server", Timeout: 30},
		Methods: []MethodDefinition{{
			Name:        name,
			LocalName:   name,
			Description: "Test method",
			Scope:       scope,
		}},
	}
}

func testRegistrationWithLocalName(name string, localName string, scope string) *Registration {
	reg := testRegistration(name, scope)
	reg.Methods[0].LocalName = localName
	return reg
}

func testRegistrationWithNames(names ...string) *Registration {
	reg := &Registration{
		Server: ServerConfig{Type: ServerTypeStdio, Command: "./server", Timeout: 30},
	}
	for _, name := range names {
		reg.Methods = append(reg.Methods, MethodDefinition{
			Name:        name,
			LocalName:   name,
			Description: "Test method",
			Scope:       ScopeShared,
		})
	}
	return reg
}

func testMCPRegistration(name string) *Registration {
	reg := testRegistration(name, ScopePrivate)
	reg.Methods[0].MCPTool = true
	return reg
}

func TestRegistryOwnerSeesBareSharedMethod(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	space := testSpace("space-1", "notes", owner.Id)
	if err := registry.Register(space, owner, testRegistration("test", ScopeShared)); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	methods := registry.List(owner)
	if len(methods) != 1 || methods[0].Name != "test" {
		t.Fatalf("expected owner to see bare method, got %#v", methods)
	}
}

func TestRegistrySharedUserSeesUserNamespace(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	space := testSpace("space-1", "notes", owner.Id)
	if err := registry.Register(space, owner, testRegistration("test", ScopeShared)); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	role := &model.Role{Id: "role-methods", Permissions: []uint16{model.PermissionUseMethods}}
	model.SetRoleCache([]*model.Role{role})
	caller := testUser("user-2", "alice", nil)
	caller.Roles = []string{role.Id}

	methods := registry.List(caller)
	if len(methods) != 1 || methods[0].Name != "user.paul.test" {
		t.Fatalf("expected shared user namespace, got %#v", methods)
	}
}

func TestRegistryRejectsDifferentDuplicate(t *testing.T) {
	registry := NewRegistry()
	owner1 := testUser("user-1", "paul", nil)
	owner2 := testUser("user-2", "alice", nil)
	if err := registry.Register(testSpace("space-1", "one", owner1.Id), owner1, testRegistration("same", ScopeShared)); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	other := testRegistration("same", ScopeShared)
	other.Methods[0].Description = "Different"
	if err := registry.Register(testSpace("space-2", "two", owner2.Id), owner2, other); err == nil {
		t.Fatalf("expected duplicate mismatch error")
	}
}

func TestRegistryRejectsDuplicateMCPToolNameAcrossMethods(t *testing.T) {
	registry := NewRegistry()
	owner1 := testUser("user-1", "paul", nil)
	owner2 := testUser("user-2", "alice", nil)
	if err := registry.Register(testSpace("space-1", "one", owner1.Id), owner1, testMCPRegistration("notes.search")); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Register(testSpace("space-2", "two", owner2.Id), owner2, testMCPRegistration("notes-search")); err == nil {
		t.Fatalf("expected duplicate MCP tool name error")
	}
}

// helper that registers a method with explicit local_name, returns the owner.
func registerDotted(t *testing.T, registry *Registry, owner *model.User, spaceName, canonicalName, localName string) {
	t.Helper()
	reg := &Registration{
		Server: ServerConfig{Type: ServerTypeStdio, Command: "./server", Timeout: 30},
		Methods: []MethodDefinition{{
			Name:        canonicalName,
			LocalName:   localName,
			Description: "Test",
			Scope:       ScopeShared,
		}},
	}
	if err := registry.Register(testSpace("space-"+spaceName, spaceName, owner.Id), owner, reg); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

func roleWithMethods() (*model.Role, *model.User) {
	role := &model.Role{Id: "role-methods", Permissions: []uint16{PermissionUseMethods}}
	model.SetRoleCache([]*model.Role{role})
	caller := testUser("user-caller", "caller", nil)
	caller.Roles = []string{role.Id}
	return role, caller
}

// Owner calls bare canonical name → routes, agent receives local_name.
func TestPickBareOwnerRoutesToLocalName(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	reg := &Registration{
		Server: ServerConfig{Type: ServerTypeStdio, Command: "./s", Timeout: 30},
		Methods: []MethodDefinition{{
			Name: "search", LocalName: "search", Description: "d", Scope: ScopeShared,
		}},
	}
	if err := registry.Register(testSpace("space-1", "notes", owner.Id), owner, reg); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	entry, localName, err := registry.Pick("search", owner)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if localName != "search" {
		t.Errorf("expected local_name %q, got %q", "search", localName)
	}
	if entry.OwnerID != owner.Id {
		t.Errorf("routed to wrong owner")
	}
}

// Non-owner calls user.<owner>.<bare> → routes, agent receives local_name.
func TestPickBareNonOwnerUserPrefixRoutesToLocalName(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	reg := &Registration{
		Server: ServerConfig{Type: ServerTypeStdio, Command: "./s", Timeout: 30},
		Methods: []MethodDefinition{{
			Name: "search", LocalName: "search", Description: "d", Scope: ScopeShared,
		}},
	}
	if err := registry.Register(testSpace("space-1", "notes", owner.Id), owner, reg); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	roleWithMethods()
	caller := testUser("user-2", "alice", nil)
	caller.Roles = []string{"role-methods"}

	entry, localName, err := registry.Pick("user.paul.search", caller)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if localName != "search" {
		t.Errorf("expected local_name %q, got %q", "search", localName)
	}
	if entry.OwnerID != owner.Id {
		t.Errorf("routed to wrong owner")
	}
}

// Owner calls dotted canonical name ({{space}}.method) → routes, agent
// receives the local_name (which strips the space prefix).
func TestPickDottedOwnerRoutesToStrippedLocalName(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	registerDotted(t, registry, owner, "method", "method.search", "search")

	entry, localName, err := registry.Pick("method.search", owner)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if localName != "search" {
		t.Errorf("expected local_name %q (stripped space prefix), got %q", "search", localName)
	}
	if entry.OwnerID != owner.Id {
		t.Errorf("routed to wrong owner")
	}
}

// Non-owner cannot call a dotted canonical name directly — they must use the
// user.<owner>.<canonical> form. The bare dotted form is reserved for the
// owner. (Used to work; now correctly rejected so display and call paths
// agree.)
func TestPickDottedNonOwnerDirectRejected(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	registerDotted(t, registry, owner, "method", "method.search", "search")

	roleWithMethods()
	caller := testUser("user-2", "alice", nil)
	caller.Roles = []string{"role-methods"}

	if _, _, err := registry.Pick("method.search", caller); err == nil {
		t.Fatalf("expected error when non-owner calls dotted canonical directly")
	}
}

// Non-owner calls user.<owner>.<dotted> → routes via prefix-strip, agent
// receives local_name. Without the fix this returned ErrMethodNotFound.
func TestPickDottedNonOwnerUserPrefixRoutesToStrippedLocalName(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	registerDotted(t, registry, owner, "method", "method.search", "search")

	roleWithMethods()
	caller := testUser("user-2", "alice", nil)
	caller.Roles = []string{"role-methods"}

	entry, localName, err := registry.Pick("user.paul.method.search", caller)
	if err != nil {
		t.Fatalf("Pick() error = %v (expected user.<owner>.<dotted> to route)", err)
	}
	if localName != "search" {
		t.Errorf("expected local_name %q, got %q", "search", localName)
	}
	if entry.OwnerID != owner.Id {
		t.Errorf("routed to wrong owner")
	}
}

// Non-owner sees shared methods (bare or dotted canonical) under the
// user.<owner>. namespace, never bare. Owner sees bare canonical only —
// never user.<self>.<name>.
func TestListSharedDottedUsesUserPrefixForNonOwner(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	registerDotted(t, registry, owner, "method", "method.search", "search")

	// Owner sees bare canonical, NOT user.paul.method.search.
	ownerList := registry.List(owner)
	if len(ownerList) != 1 {
		t.Fatalf("owner: expected 1 method, got %d", len(ownerList))
	}
	if ownerList[0].Name != "method.search" {
		t.Errorf("owner: expected bare canonical %q, got %q", "method.search", ownerList[0].Name)
	}

	// Non-owner sees user.paul.method.search, NOT bare method.search.
	roleWithMethods()
	caller := testUser("user-2", "alice", nil)
	caller.Roles = []string{"role-methods"}

	callerList := registry.List(caller)
	if len(callerList) != 1 {
		t.Fatalf("caller: expected 1 method, got %d", len(callerList))
	}
	if callerList[0].Name != "user.paul.method.search" {
		t.Errorf("caller: expected %q, got %q", "user.paul.method.search", callerList[0].Name)
	}
}

// Sanity check: user.<wrong-owner>.<dotted> doesn't accidentally route to a
// different owner who happens to own the same dotted name.
func TestPickDottedUserPrefixDoesNotCrossMatchOwners(t *testing.T) {
	registry := NewRegistry()
	paul := testUser("user-1", "paul", nil)
	alice := testUser("user-2", "alice", nil)
	registerDotted(t, registry, paul, "method1", "method.search", "search")
	registerDotted(t, registry, alice, "method2", "method.search", "search")

	roleWithMethods()
	caller := testUser("user-3", "bob", nil)
	caller.Roles = []string{"role-methods"}

	entry, _, err := registry.Pick("user.paul.method.search", caller)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if entry.OwnerID != paul.Id {
		t.Errorf("expected routing pinned to paul, got owner_id=%s", entry.OwnerID)
	}
}

// helper that registers a method owned by `owner` with a group filter.
func registerGroupFiltered(t *testing.T, registry *Registry, owner *model.User, spaceName, canonicalName, localName string, groups []string) {
	t.Helper()
	reg := &Registration{
		Server: ServerConfig{Type: ServerTypeStdio, Command: "./server", Timeout: 30},
		Methods: []MethodDefinition{{
			Name:        canonicalName,
			LocalName:   localName,
			Description: "Test",
			Scope:       ScopeShared,
			Groups:      groups,
		}},
	}
	if err := registry.Register(testSpace("space-"+spaceName, spaceName, owner.Id), owner, reg); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

// Caller who fails the group filter on a SHARED method gets 403, not 404 —
// both via the direct canonical form and the user.<owner>.<canonical> form.
// Returning 404 for "exists but you can't access it" leaks method existence
// through a 404-vs-403 oracle and is inconsistent between call forms.
func TestPickGroupFilteredReturnsPermissionNotNotFound(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	// Method scoped shared, restricted to a group the caller isn't in. Use a
	// realistic group ID since that's what the registry stores after
	// resolveMethodGroups has run on the server side.
	registerGroupFiltered(t, registry, owner, "notes", "notes.search", "search", []string{"g-allowed"})

	roleWithMethods()
	caller := testUser("user-2", "alice", nil)
	caller.Roles = []string{"role-methods"} // has UseMethods, but no groups

	// Direct canonical form — pre-existing behavior, must stay 403.
	if _, _, err := registry.Pick("notes.search", caller); !errors.Is(err, ErrPermission) {
		t.Errorf("direct canonical: expected ErrPermission, got %v", err)
	}

	// Namespaced form — was 404 before the fix, must now be 403 too.
	if _, _, err := registry.Pick("user.paul.notes.search", caller); !errors.Is(err, ErrPermission) {
		t.Errorf("namespaced form: expected ErrPermission, got %v", err)
	}
}

// A genuinely unknown method (no matching canonical, no matching owner in the
// user. namespace) still returns 404 — the fix doesn't paper over real
// not-found cases.
func TestPickUnknownMethodStillReturnsNotFound(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)
	registerDotted(t, registry, owner, "notes", "notes.search", "search")

	roleWithMethods()
	caller := testUser("user-2", "alice", nil)
	caller.Roles = []string{"role-methods"}

	// Totally unknown canonical.
	if _, _, err := registry.Pick("does.not.exist", caller); !errors.Is(err, ErrMethodNotFound) {
		t.Errorf("unknown canonical: expected ErrMethodNotFound, got %v", err)
	}

	// user.<owner>.<canonical> where the owner has no such method.
	if _, _, err := registry.Pick("user.paul.does.not.exist", caller); !errors.Is(err, ErrMethodNotFound) {
		t.Errorf("unknown namespaced: expected ErrMethodNotFound, got %v", err)
	}

	// user.<wrong-owner>.<canonical> where the canonical exists for a
	// different owner. Still 404, not 403 — the named owner doesn't have it.
	if _, _, err := registry.Pick("user.nobody.notes.search", caller); !errors.Is(err, ErrMethodNotFound) {
		t.Errorf("unknown owner namespaced: expected ErrMethodNotFound, got %v", err)
	}
}

func TestPickRoundRobinsSequentialCallsAcrossDuplicateMethods(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)

	if err := registry.Register(testSpace("space-1", "worker-1", owner.Id), owner, testRegistration("search", ScopeShared)); err != nil {
		t.Fatalf("Register() space-1 error = %v", err)
	}
	if err := registry.Register(testSpace("space-2", "worker-2", owner.Id), owner, testRegistration("search", ScopeShared)); err != nil {
		t.Fatalf("Register() space-2 error = %v", err)
	}
	if err := registry.Register(testSpace("space-3", "worker-3", owner.Id), owner, testRegistration("search", ScopeShared)); err != nil {
		t.Fatalf("Register() space-3 error = %v", err)
	}

	var got []string
	for i := 0; i < 6; i++ {
		entry, _, err := registry.Pick("search", owner)
		if err != nil {
			t.Fatalf("Pick() %d error = %v", i, err)
		}
		got = append(got, entry.SpaceID)
		registry.Done(entry)
	}

	want := []string{"space-1", "space-2", "space-3", "space-1", "space-2", "space-3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("round robin order = %v, want %v", got, want)
		}
	}
}

func TestPickUsesLeastInflightBeforeRoundRobinTieBreak(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)

	if err := registry.Register(testSpace("space-1", "worker-1", owner.Id), owner, testRegistration("search", ScopeShared)); err != nil {
		t.Fatalf("Register() space-1 error = %v", err)
	}
	if err := registry.Register(testSpace("space-2", "worker-2", owner.Id), owner, testRegistration("search", ScopeShared)); err != nil {
		t.Fatalf("Register() space-2 error = %v", err)
	}

	first, _, err := registry.Pick("search", owner)
	if err != nil {
		t.Fatalf("Pick() first error = %v", err)
	}
	second, _, err := registry.Pick("search", owner)
	if err != nil {
		t.Fatalf("Pick() second error = %v", err)
	}
	third, _, err := registry.Pick("search", owner)
	if err != nil {
		t.Fatalf("Pick() third error = %v", err)
	}
	defer registry.Done(first)
	defer registry.Done(second)
	defer registry.Done(third)

	if first.SpaceID != "space-1" {
		t.Fatalf("first pick = %s, want space-1", first.SpaceID)
	}
	if second.SpaceID != "space-2" {
		t.Fatalf("second pick = %s, want space-2", second.SpaceID)
	}
	if third.SpaceID != "space-1" {
		t.Fatalf("third pick = %s, want least-inflight round robin to return space-1, got %s", third.SpaceID, third.SpaceID)
	}
}

func TestPickRoundRobinsNamespacedDuplicateMethods(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)

	if err := registry.Register(testSpace("space-1", "worker-1", owner.Id), owner, testRegistrationWithLocalName("notes.search", "search", ScopeShared)); err != nil {
		t.Fatalf("Register() space-1 error = %v", err)
	}
	if err := registry.Register(testSpace("space-2", "worker-2", owner.Id), owner, testRegistrationWithLocalName("notes.search", "search", ScopeShared)); err != nil {
		t.Fatalf("Register() space-2 error = %v", err)
	}

	roleWithMethods()
	caller := testUser("user-2", "alice", nil)
	caller.Roles = []string{"role-methods"}

	first, localName, err := registry.Pick("user.paul.notes.search", caller)
	if err != nil {
		t.Fatalf("Pick() first error = %v", err)
	}
	if localName != "search" {
		t.Fatalf("first localName = %q, want search", localName)
	}
	registry.Done(first)

	second, localName, err := registry.Pick("user.paul.notes.search", caller)
	if err != nil {
		t.Fatalf("Pick() second error = %v", err)
	}
	if localName != "search" {
		t.Fatalf("second localName = %q, want search", localName)
	}
	registry.Done(second)

	if first.SpaceID != "space-1" || second.SpaceID != "space-2" {
		t.Fatalf("namespaced round robin picked %s then %s, want space-1 then space-2", first.SpaceID, second.SpaceID)
	}
}

func TestRegisterMixedUniqueAndDuplicateMethods(t *testing.T) {
	registry := NewRegistry()
	owner := testUser("user-1", "paul", nil)

	if err := registry.Register(
		testSpace("space-1", "worker-1", owner.Id),
		owner,
		testRegistrationWithNames("method-1", "method-2", "method-3", "method-4", "method-5", "method-6", "method-7", "method-8", "method-9", "method-10"),
	); err != nil {
		t.Fatalf("Register() space-1 error = %v", err)
	}
	if err := registry.Register(
		testSpace("space-2", "worker-2", owner.Id),
		owner,
		testRegistrationWithNames("method-3", "method-7"),
	); err != nil {
		t.Fatalf("Register() space-2 duplicate subset error = %v", err)
	}

	list := registry.List(owner)
	if len(list) != 10 {
		t.Fatalf("List() returned %d methods, want 10: %#v", len(list), list)
	}

	unique, _, err := registry.Pick("method-1", owner)
	if err != nil {
		t.Fatalf("Pick() unique error = %v", err)
	}
	registry.Done(unique)
	if unique.SpaceID != "space-1" {
		t.Fatalf("unique method routed to %s, want space-1", unique.SpaceID)
	}

	firstDuplicate, _, err := registry.Pick("method-3", owner)
	if err != nil {
		t.Fatalf("Pick() first duplicate error = %v", err)
	}
	registry.Done(firstDuplicate)
	secondDuplicate, _, err := registry.Pick("method-3", owner)
	if err != nil {
		t.Fatalf("Pick() second duplicate error = %v", err)
	}
	registry.Done(secondDuplicate)
	if firstDuplicate.SpaceID != "space-1" || secondDuplicate.SpaceID != "space-2" {
		t.Fatalf("duplicate method routed to %s then %s, want space-1 then space-2", firstDuplicate.SpaceID, secondDuplicate.SpaceID)
	}

	otherDuplicate, _, err := registry.Pick("method-7", owner)
	if err != nil {
		t.Fatalf("Pick() other duplicate error = %v", err)
	}
	registry.Done(otherDuplicate)
	if otherDuplicate.SpaceID != "space-1" {
		t.Fatalf("other duplicate first pick routed to %s, want space-1", otherDuplicate.SpaceID)
	}
}
