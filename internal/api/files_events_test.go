package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/filestore"
	"github.com/paularlott/knot/internal/sse"
)

func TestNotifyFilesChangedPerUser(t *testing.T) {
	_, store := filesEnv(t)
	hub := sse.GetHub()
	hub.Start()

	alice := hub.NewClient("fs-alice", "files-events-a")
	bob := hub.NewClient("fs-bob", "files-events-b")
	stranger := hub.NewClient("fs-nobody", "files-events-c")
	defer alice.Close()
	defer bob.Close()
	defer stranger.Close()

	pa := &filestore.Principal{UserId: "fs-alice", Username: "scriptalice", CanOwn: true, CanShare: true}
	mine, err := store.CreateBucket(pa, "scriptalice--mine")
	if err != nil {
		t.Fatal(err)
	}
	shared, err := store.CreateBucket(pa, "scriptalice--shared")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetGrant(pa, "scriptalice--shared", filestore.Grant{Type: filestore.GrantUser, Id: "fs-bob", Access: filestore.GrantRead}); err != nil {
		t.Fatal(err)
	}

	var got func(c *sse.Client) []string
	got = func(c *sse.Client) []string {
		t.Helper()
		select {
		case data := <-c.Send():
			var ev struct {
				Type    string
				Payload sse.FilesPayload
			}
			json.Unmarshal(data, &ev)
			if ev.Type != "files:changed" {
				return got(c) // another kind of event
			}
			return ev.Payload.BucketIds
		case <-time.After(300 * time.Millisecond):
			return nil
		}
	}
	drain := func(c *sse.Client) {
		for {
			select {
			case <-c.Send():
			case <-time.After(100 * time.Millisecond):
				return
			}
		}
	}
	for _, c := range []*sse.Client{alice, bob, stranger} {
		drain(c)
	}

	NotifyFilesChanged([]string{mine.Id, shared.Id})
	if ids := got(alice); len(ids) != 2 {
		t.Errorf("alice heard of %v", ids)
	}
	if ids := got(bob); len(ids) != 1 || ids[0] != shared.Id {
		t.Errorf("bob heard of %v", ids)
	}
	if ids := got(stranger); ids != nil {
		t.Errorf("a stranger heard of %v", ids)
	}

	// Unshared: bob hears of it once more, so his view drops the bucket.
	if _, err := store.RemoveGrant(pa, "scriptalice--shared", filestore.GrantUser, "fs-bob"); err != nil {
		t.Fatal(err)
	}
	NotifyFilesChanged([]string{shared.Id})
	if ids := got(bob); len(ids) != 1 {
		t.Errorf("bob not told of the unshare: %v", ids)
	}
}
