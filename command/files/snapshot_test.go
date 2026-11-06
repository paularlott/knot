package command_files

import (
	"context"
	"strings"
	"testing"
)

func TestSnapshot(t *testing.T) {
	client, store := newFilesServer(t)
	testBucket(t, store, "other")
	ctx := context.Background()

	putRemote(t, client, "sync-test", "proj/a.go", "package a")
	putRemote(t, client, "sync-test", "proj/sub/b.go", "package b")
	putRemote(t, client, "sync-test", "projx", "beside")

	if err := runSnapshot(ctx, client, "sync-test:proj/", "other:snaps/one/"); err != nil {
		t.Fatal(err)
	}
	expectKeysWithPrefixIn(t, client, "other", "snaps/one/", "snaps/one/.knot-snapshot.json", "snaps/one/a.go", "snaps/one/sub/b.go")
	if c, _ := remoteContent(t, client, "other", "snaps/one/sub/b.go"); c != "package b" {
		t.Errorf("snapshot file reads %q", c)
	}

	// The whole bucket, by its full name.
	if err := runSnapshot(ctx, client, "tester--sync-test:", "sync-test-copy-free:"); err == nil {
		t.Error("a snapshot with no new folder was made")
	}
	if err := runSnapshot(ctx, client, "tester--sync-test:", "other:all"); err != nil {
		t.Fatal(err)
	}
	if c, _ := remoteContent(t, client, "other", "all/projx"); c != "beside" {
		t.Errorf("whole bucket snapshot reads %q", c)
	}

	for _, c := range []struct{ from, to, want string }{
		{"sync-test:proj", "other:snaps/one", "already exists"},
		{"sync-test:proj/*.go", "other:x", "not a pattern"},
		{"sync-test:proj", "./local", "not a bucket path"},
		{"sync-test:gone", "other:x", "not found"},
	} {
		if err := runSnapshot(ctx, client, c.from, c.to); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("snapshot %s %s: %v, want an error containing %q", c.from, c.to, err, c.want)
		}
	}
}
