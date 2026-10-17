package cluster

import (
	"testing"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
)

// A deletion wins over a live copy whatever the timestamps: a server that
// refreshed a session before hearing of its logout gossips a newer live
// copy, which mustn't revive it, nor make a server that took the refresh
// first ignore the older deletion.
func TestMergeSessionsDeletionWins(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()}})
	t.Cleanup(func() { config.SetServerConfig(prev) })
	store := database.GetSessionStorage()
	c := &Cluster{logger: log.WithGroup("cluster")}

	session := func(id string, deleted bool, at hlc.Timestamp) *model.Session {
		return &model.Session{Id: id, UserId: "u", IsDeleted: deleted, UpdatedAt: at, ExpiresAfter: time.Now().Add(time.Hour).UTC()}
	}
	isDeleted := func(id string) bool {
		s, err := store.GetSession(id)
		if err != nil || s == nil {
			t.Fatalf("session %s: %v, %v", id, s, err)
		}
		return s.IsDeleted
	}

	// Logged out here, then a newer live copy arrives from a server that
	// refreshed it before hearing.
	deletedAt := hlc.Now()
	refreshedAt := hlc.Now()
	if err := store.SaveSession(session("a", true, deletedAt)); err != nil {
		t.Fatal(err)
	}
	if err := c.mergeSessions([]*model.Session{session("a", false, refreshedAt)}); err != nil {
		t.Fatal(err)
	}
	if !isDeleted("a") {
		t.Error("a newer live copy revived a deleted session")
	}

	// This server took the refresh first; the older deletion still applies.
	if err := store.SaveSession(session("b", false, refreshedAt)); err != nil {
		t.Fatal(err)
	}
	if err := c.mergeSessions([]*model.Session{session("b", true, deletedAt)}); err != nil {
		t.Fatal(err)
	}
	if !isDeleted("b") {
		t.Error("an older deletion was ignored")
	}

	// Ordinary refreshes still merge by timestamp.
	older, newer := hlc.Now(), hlc.Now()
	if err := store.SaveSession(session("c", false, older)); err != nil {
		t.Fatal(err)
	}
	refreshed := session("c", false, newer)
	refreshed.ExpiresAfter = time.Now().Add(2 * time.Hour).UTC()
	if err := c.mergeSessions([]*model.Session{refreshed}); err != nil {
		t.Fatal(err)
	}
	if s, _ := store.GetSession("c"); s == nil || s.UpdatedAt != newer || s.IsDeleted {
		t.Errorf("newer refresh not merged: %+v", s)
	}
}
