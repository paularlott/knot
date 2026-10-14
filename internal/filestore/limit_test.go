package filestore

import (
	"errors"
	"strings"
	"testing"
)

// limited is a principal as a restricted space sees file storage: only the
// buckets in caps, at most at the level given.
func limited(p *Principal, caps map[string]int) *Principal {
	c := *p
	c.Limit = func(bucketId string) int { return caps[bucketId] }
	return &c
}

func bucketId(t *testing.T, s *Store, p *Principal, name string) string {
	t.Helper()
	info, err := s.GetBucket(p, name)
	if err != nil {
		t.Fatalf("get %s: %v", name, err)
	}
	return info.Id
}

func TestLimitedPrincipal(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "notes")
	mustCreate(t, s, alice, "scratch")
	mustCreate(t, s, alice, "private")
	put(t, s, alice, "notes", "a.txt", "notes")
	put(t, s, alice, "private", "a.txt", "secret")

	p := limited(alice, map[string]int{
		bucketId(t, s, alice, "notes"):   AccessRead,
		bucketId(t, s, alice, "scratch"): AccessWrite,
	})

	// Only the allowed buckets are listed, and none as owner.
	var names []string
	for _, b := range s.ListBuckets(p, true) {
		names = append(names, b.Name)
		if b.Access > AccessWrite {
			t.Errorf("%s listed with owner access", b.Name)
		}
	}
	if strings.Join(names, ",") != "notes,scratch" {
		t.Fatalf("listed %v, want notes and scratch", names)
	}

	// Hidden buckets don't exist to it.
	if _, err := s.GetBucket(p, "private"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("get private: got %v, want no such bucket", err)
	}
	if _, _, err := s.OpenObject(t.Context(), p, "private", "a.txt"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("read private: got %v, want no such bucket", err)
	}

	// Read-only buckets read but don't write.
	if got := read(t, s, p, "notes", "a.txt"); got != "notes" {
		t.Errorf("read notes = %q", got)
	}
	if _, err := s.PutObject(p, "notes", "b.txt", strings.NewReader("x"), PutOptions{Size: 1}); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("write read-only notes: got %v, want access denied", err)
	}
	if err := s.DeleteObject(p, "notes", "a.txt"); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("delete in read-only notes: got %v, want access denied", err)
	}

	// Writable buckets write.
	put(t, s, p, "scratch", "b.txt", "ok")

	// It never manages buckets, even its owner's.
	if _, err := s.CreateBucket(p, "new"); err == nil {
		t.Error("a limited principal created a bucket")
	}
	if err := s.DeleteBucket(p, "scratch", true); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("delete scratch: got %v, want access denied", err)
	}
	if _, err := s.SetGrant(p, "scratch", Grant{Type: GrantUser, Id: "bob", Access: GrantRead}); err == nil {
		t.Error("a limited principal shared a bucket")
	}
	if _, err := s.TransferBucket(p, "scratch", "bob", "bob", false); err == nil {
		t.Error("a limited principal transferred a bucket")
	}
}

func TestLimitedAdminLosesBlanketAccess(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "notes")
	p := limited(admin, map[string]int{})
	if got := s.ListBuckets(p, true); len(got) != 0 {
		t.Fatalf("a limited administrator listed %d buckets", len(got))
	}
	if _, err := s.GetBucket(p, "notes"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("get notes: got %v, want no such bucket", err)
	}
}

func TestLimitDoesNotRaiseAccess(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "shared")
	if _, err := s.SetGrant(alice, "shared", Grant{Type: GrantUser, Id: "bob", Access: GrantRead}); err != nil {
		t.Fatal(err)
	}
	p := limited(bob, map[string]int{bucketId(t, s, alice, "shared"): AccessWrite})
	if _, err := s.PutObject(p, "shared", "x", strings.NewReader("x"), PutOptions{Size: 1}); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("write a bucket shared read-only: got %v, want access denied", err)
	}
}
