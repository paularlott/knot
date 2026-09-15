package service

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/validate"
)

func TestStackKeyAlias(t *testing.T) {
	cases := []struct{ in, want string }{
		{"space-1", "space_1"},
		{"db", "db"}, // no hyphen -> unchanged
		{"a-b-c", "a_b_c"},
		{"my-db", "my_db"},
	}
	for _, c := range cases {
		if got := stackKeyAlias(c.in); got != c.want {
			t.Fatalf("stackKeyAlias(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestStackAliasInvariant documents the load-bearing assumption that makes the
// dotted "-" -> "_" alias collision-free: space names (and thus stack keys) can
// never contain "_". If this ever changes, the aliasing in BuildStackVariableData
// could let two distinct siblings shadow one another, so this test must keep
// passing (or the aliasing strategy must be revisited).
func TestStackAliasInvariant(t *testing.T) {
	if validate.Name("has_underscore") {
		t.Fatal("space names must not allow '_' (URL safety); the .stack dotted alias depends on this")
	}
	if !validate.Name("has-hyphen") {
		t.Fatal("expected hyphenated names to be valid")
	}
}

// Sibling entries must expose the space's IP address so mixed stacks can
// wire a container to a bridged KVM sibling's static address.
func TestStackSiblingIPAddress(t *testing.T) {
	// BuildStackVariableData reads siblings from the database.
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	db := database.GetInstance()
	user := newTestUser(t)

	vm := &model.Space{Id: "vm-id", Name: "db", Stack: "mystack", StackPrefix: "my", TemplateHash: "h", IPAddress: "192.0.2.10"}
	app := &model.Space{Id: "app-id", Name: "web", Stack: "mystack", StackPrefix: "my", TemplateHash: "h"}

	vm.UserId = user.Id
	app.UserId = user.Id
	if err := db.SaveSpace(vm, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSpace(app, nil); err != nil {
		t.Fatal(err)
	}

	data := BuildStackVariableData(app, nil)
	if data == nil {
		t.Fatal("expected stack data")
	}
	dbEntry, ok := data["db"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected sibling entry for db, got %T", data["db"])
	}
	space, _ := dbEntry["space"].(map[string]interface{})
	if space == nil || space["ip_address"] != "192.0.2.10" {
		t.Fatalf("sibling ip_address missing or wrong: %+v", space)
	}

	// Containers carry an empty address rather than omitting the key.
	data = BuildStackVariableData(vm, nil)
	web, _ := data["web"].(map[string]interface{})
	space, _ = web["space"].(map[string]interface{})
	if space == nil || space["ip_address"] != "" {
		t.Fatalf("container sibling should carry an empty ip_address: %+v", space)
	}
}
