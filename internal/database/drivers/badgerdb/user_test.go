package driver_badgerdb

import (
	"strings"
	"testing"

	"github.com/dgraph-io/badger/v4"

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

// Provider tokens earlier versions stored with a linked provider go when the
// user is saved again, as the drop_provider_tokens data migration does.
func TestSaveUserDropsLegacyProviderTokens(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()}, Zone: "test"})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	db := &BadgerDbDriver{}
	if err := db.Connect(); err != nil {
		t.Fatal(err)
	}
	user := &model.User{Id: "u1", Username: "owner", Email: "owner@example.com", Active: true,
		ExternalAuthProviders: map[string]model.ExternalProvider{"google": {ProviderUID: "g1", Username: "o"}}}
	if err := db.SaveUser(user, nil); err != nil {
		t.Fatal(err)
	}

	// Store it as an earlier version did, with the tokens.
	legacy := `"google":{"provider_uid":"g1","username":"o","token":"enc-access","refresh_token":"enc-refresh"}`
	raw := func() string {
		var v string
		db.connection.View(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte("Users:u1"))
			if err != nil {
				return err
			}
			return item.Value(func(b []byte) error { v = string(b); return nil })
		})
		return v
	}
	stored := strings.Replace(raw(), `"google":{"provider_uid":"g1","username":"o"}`, legacy, 1)
	if !strings.Contains(stored, "enc-refresh") {
		t.Fatalf("couldn't plant legacy tokens in %s", stored)
	}
	if err := db.connection.Update(func(txn *badger.Txn) error { return txn.Set([]byte("Users:u1"), []byte(stored)) }); err != nil {
		t.Fatal(err)
	}

	loaded, err := db.GetUser("u1")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveUser(loaded, []string{"ExternalAuthProviders"}); err != nil {
		t.Fatal(err)
	}
	if v := raw(); strings.Contains(v, "enc-access") || strings.Contains(v, "enc-refresh") || !strings.Contains(v, `"provider_uid":"g1"`) {
		t.Errorf("stored user after rewrite = %s", v)
	}
}
