package driver_memory

import (
	"testing"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

// TestSaveSessionUserIndexFollowsSwitch pins the by-user index maintenance:
// switching a session to another user (then flicking back) must move the
// index entry with it. Before this, the entry stayed under the login user
// forever, so per-user session listings showed sessions the user no longer
// ran as — and any per-user session sweep hit the wrong ones.
func TestSaveSessionUserIndexFollowsSwitch(t *testing.T) {
	db := &MemoryDbDriver{}
	db.Connect()

	// Login as a; the switch handler rewrites UserId in place and saves.
	login := &model.Session{Id: "s1", UserId: "a", ExpiresAfter: time.Now().Add(time.Hour).UTC()}
	if err := db.SaveSession(login); err != nil {
		t.Fatal(err)
	}

	switched := *login
	switched.UserId = "b"
	switched.OriginalUserId = "a"
	if err := db.SaveSession(&switched); err != nil {
		t.Fatal(err)
	}
	for _, s := range mustSessionsForUser(t, db, "a") {
		if s.Id == "s1" {
			t.Error("session still indexed under the login user after switching to b")
		}
	}
	if len(mustSessionsForUser(t, db, "b")) != 1 {
		t.Errorf("sessions under b = %d, want the switched session", len(mustSessionsForUser(t, db, "b")))
	}

	// Flick back: the b index entry must go away again.
	back := switched
	back.UserId = "a"
	if err := db.SaveSession(&back); err != nil {
		t.Fatal(err)
	}
	if len(mustSessionsForUser(t, db, "a")) != 1 {
		t.Errorf("sessions under a = %d, want the returned session", len(mustSessionsForUser(t, db, "a")))
	}
	for _, s := range mustSessionsForUser(t, db, "b") {
		if s.Id == "s1" {
			t.Error("session lingered under b after flicking back")
		}
	}

	// Unrelated saves keep the index stable.
	if err := db.SaveSession(&back); err != nil {
		t.Fatal(err)
	}
	if len(mustSessionsForUser(t, db, "a")) != 1 || len(mustSessionsForUser(t, db, "b")) != 0 {
		t.Error("index changed on a same-user save")
	}
}

func mustSessionsForUser(t *testing.T, db *MemoryDbDriver, userId string) []*model.Session {
	t.Helper()
	sessions, err := db.GetSessionsForUser(userId)
	if err != nil {
		t.Fatal(err)
	}
	return sessions
}
