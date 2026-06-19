package agent_server

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/methods"
)

// fakeGroupsDB satisfies enough of database.DbDriver for resolveMethodGroups.
// Other methods panic if called — the resolver only needs GetGroups.
type fakeGroupsDB struct {
	database.DbDriver // embedded nil so we only override what we use
	groups            []*model.Group
}

func (f *fakeGroupsDB) GetGroups() ([]*model.Group, error) {
	return f.groups, nil
}

func TestResolveMethodGroupsAcceptsNamesAndIDs(t *testing.T) {
	db := &fakeGroupsDB{groups: []*model.Group{
		{Id: "g-3b", Name: "Group 3b"},
		{Id: "g-admin", Name: "admins"},
	}}
	reg := &methods.Registration{
		Methods: []methods.MethodDefinition{
			{
				Name:   "search",
				Scope:  methods.ScopeShared,
				Groups: []string{"Group 3b", "g-admin"}, // name then ID
			},
		},
	}
	if err := resolveMethodGroups(db, reg); err != nil {
		t.Fatalf("resolveMethodGroups: %v", err)
	}
	got := reg.Methods[0].Groups
	if len(got) != 2 || got[0] != "g-3b" || got[1] != "g-admin" {
		t.Errorf("expected [g-3b g-admin], got %v", got)
	}
}

func TestResolveMethodGroupsRejectsUnknown(t *testing.T) {
	db := &fakeGroupsDB{groups: []*model.Group{
		{Id: "g-3b", Name: "Group 3b"},
	}}
	reg := &methods.Registration{
		Methods: []methods.MethodDefinition{
			{
				Name:   "search",
				Scope:  methods.ScopeShared,
				Groups: []string{"Nonexistent"},
			},
		},
	}
	err := resolveMethodGroups(db, reg)
	if err == nil {
		t.Fatalf("expected error for unknown group, got nil")
	}
	if !strings.Contains(err.Error(), "Nonexistent") {
		t.Errorf("error should name the unknown group, got: %v", err)
	}
}

func TestResolveMethodGroupsNoopWhenEmpty(t *testing.T) {
	db := &fakeGroupsDB{} // no groups table needed
	reg := &methods.Registration{
		Methods: []methods.MethodDefinition{
			{Name: "search", Scope: methods.ScopeShared, Groups: nil},
		},
	}
	// Should not call GetGroups at all — fast path.
	if err := resolveMethodGroups(db, reg); err != nil {
		t.Fatalf("expected nil for groups-less registration, got %v", err)
	}
}

func TestResolveMethodGroupsDedupesAndStripsEmpty(t *testing.T) {
	db := &fakeGroupsDB{groups: []*model.Group{
		{Id: "g-3b", Name: "Group 3b"},
	}}
	reg := &methods.Registration{
		Methods: []methods.MethodDefinition{
			{
				Name:   "search",
				Scope:  methods.ScopeShared,
				Groups: []string{"", "Group 3b", ""},
			},
		},
	}
	if err := resolveMethodGroups(db, reg); err != nil {
		t.Fatalf("resolveMethodGroups: %v", err)
	}
	got := reg.Methods[0].Groups
	if len(got) != 1 || got[0] != "g-3b" {
		t.Errorf("expected [g-3b] after stripping empties, got %v", got)
	}
}
