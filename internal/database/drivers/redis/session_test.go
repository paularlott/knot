package driver_redis

import (
	"testing"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database/model"
)

// A deleted session stays deleted: a request that read it before a logout
// on another server saving a live copy afterwards must not revive it.
func TestSaveSessionKeepsDeletion(t *testing.T) {
	db := newLiveDriver(t)

	live := model.Session{Id: "s1", UserId: "a", ExpiresAfter: time.Now().Add(time.Hour).UTC(), UpdatedAt: hlc.Now()}
	if err := db.SaveSession(&live); err != nil {
		t.Fatal(err)
	}
	if got, err := db.GetSession("s1"); err != nil || got == nil || got.IsDeleted {
		t.Fatalf("live session = %+v, %v", got, err)
	}

	deleted := live
	deleted.IsDeleted = true
	deleted.UpdatedAt = hlc.Now()
	if err := db.SaveSession(&deleted); err != nil {
		t.Fatal(err)
	}

	late := live // read before the logout, refreshed after it
	late.ExpiresAfter = time.Now().Add(2 * time.Hour).UTC()
	late.UpdatedAt = hlc.Now()
	if err := db.SaveSession(&late); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession("s1")
	if err != nil || got == nil || !got.IsDeleted {
		t.Fatalf("session after a late refresh = %+v, %v, want it still deleted", got, err)
	}

	// A deletion still saves over a deletion.
	again := deleted
	again.UpdatedAt = hlc.Now()
	if err := db.SaveSession(&again); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetSession("s1"); got == nil || !got.IsDeleted || got.UpdatedAt != again.UpdatedAt {
		t.Errorf("second deletion not saved: %+v", got)
	}
}
