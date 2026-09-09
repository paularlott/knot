package api

import (
	"testing"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// TestRevokeSwitchedSessions pins the unlink revocation: only sessions the
// linking user's logins currently have switched into the unlinked account
// die — the linked user's own sessions, other linkers' switched sessions,
// and sessions switched into other accounts survive.
func TestRevokeSwitchedSessions(t *testing.T) {
	// Driver init is lazy and global; make sure it lands on a throwaway
	// badger store rather than "no database enabled".
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	store := database.GetSessionStorage()

	mk := func(id, userId, originalUserId string) *model.Session {
		return &model.Session{
			Id:             id,
			UserId:         userId,
			OriginalUserId: originalUserId,
			ExpiresAfter:   time.Now().Add(time.Hour).UTC(),
		}
	}

	switched := mk("s1", "b", "a") // the linker's live switched session — must die
	own := mk("s2", "b", "")       // the linked user's own login — untouched
	other := mk("s3", "b", "c")    // a different linker's switched session — untouched
	els := mk("s4", "d", "a")      // switched into a different account — untouched
	for _, s := range []*model.Session{switched, own, other, els} {
		if err := store.SaveSession(s); err != nil {
			t.Fatal(err)
		}
	}

	revoked := revokeSwitchedSessions("a", "b")
	if len(revoked) != 1 || revoked[0].Id != "s1" {
		t.Fatalf("revoked = %+v, want only s1", revoked)
	}

	for id, wantDead := range map[string]bool{"s1": true, "s2": false, "s3": false, "s4": false} {
		s, err := store.GetSession(id)
		if err != nil || s == nil {
			t.Fatalf("session %s missing: %v", id, err)
		}
		if s.IsDeleted != wantDead {
			t.Errorf("session %s IsDeleted = %v, want %v", id, s.IsDeleted, wantDead)
		}
	}
}
