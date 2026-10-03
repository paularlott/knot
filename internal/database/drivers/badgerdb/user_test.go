package driver_badgerdb

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

// Usernames are unique regardless of case, and a lookup finds the user
// whatever case it is given in.
func TestGetUserByUsernameIgnoresCase(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()}, Zone: "test"})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	db := &BadgerDbDriver{}
	if err := db.Connect(); err != nil {
		t.Fatal(err)
	}

	user := &model.User{Id: "u1", Username: "Owner.One", Email: "one@example.com", Active: true}
	if err := db.SaveUser(user, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Owner.One", "owner.one", "OWNER.ONE"} {
		if u, err := db.GetUserByUsername(name); err != nil || u == nil || u.Id != "u1" {
			t.Errorf("GetUserByUsername(%q) = %v, %v", name, u, err)
		}
	}
	if err := db.SaveUser(&model.User{Id: "u2", Username: "owner.ONE", Email: "two@example.com"}, nil); err == nil {
		t.Errorf("a username differing only in case was accepted")
	}
}
