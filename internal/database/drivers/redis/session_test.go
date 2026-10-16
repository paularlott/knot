package driver_redis

import (
	"context"
	"strings"
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

// Provider tokens earlier versions stored with a linked provider go when the
// user is saved again, as the drop_provider_tokens data migration does.
func TestSaveUserDropsLegacyProviderTokens(t *testing.T) {
	db := newLiveDriver(t)
	user := &model.User{Id: "u1", Username: "owner", Email: "owner@example.com", Active: true,
		ExternalAuthProviders: map[string]model.ExternalProvider{"google": {ProviderUID: "g1", Username: "o"}}}
	if err := db.SaveUser(user, nil); err != nil {
		t.Fatal(err)
	}
	key := db.prefix + "Users:u1"
	raw, err := db.get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	legacy := strings.Replace(raw, `"google":{"provider_uid":"g1","username":"o"}`,
		`"google":{"provider_uid":"g1","username":"o","token":"enc-access","refresh_token":"enc-refresh"}`, 1)
	if legacy == raw {
		t.Fatalf("couldn't plant legacy tokens in %s", raw)
	}
	if err := db.set(context.Background(), key, legacy, 0); err != nil {
		t.Fatal(err)
	}

	loaded, err := db.GetUser("u1")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveUser(loaded, []string{"ExternalAuthProviders"}); err != nil {
		t.Fatal(err)
	}
	if v, _ := db.get(context.Background(), key); strings.Contains(v, "enc-access") || strings.Contains(v, "enc-refresh") || !strings.Contains(v, `"provider_uid":"g1"`) {
		t.Errorf("stored user after rewrite = %s", v)
	}
}
