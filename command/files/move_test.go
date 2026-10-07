package command_files

import (
	"context"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

func TestMove(t *testing.T) {
	client, store := newFilesServer(t)
	testBucket(t, store, "other")
	ctx := context.Background()
	move := func(overwrite bool, args ...string) error { return runMove(ctx, client, args, overwrite) }
	mustMv := func(overwrite bool, args ...string) {
		t.Helper()
		if err := move(overwrite, args...); err != nil {
			t.Fatalf("mv %v: %v", args, err)
		}
	}
	fails := func(want string, overwrite bool, args ...string) {
		t.Helper()
		if err := move(overwrite, args...); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("mv %v: error %v, want one containing %q", args, err, want)
		}
	}

	putRemote(t, client, "sync-test", "a.txt", "alpha")
	putRemote(t, client, "sync-test", "logs/app.log", "app")
	putRemote(t, client, "sync-test", "logs/old/one.log", "one")
	putRemote(t, client, "sync-test", "logs/old/two.tmp", "two")
	putRemote(t, client, "sync-test", "keep.txt", "keep")
	putRemote(t, client, "sync-test", "empty/", "")

	// Rename.
	mustMv(false, "sync-test:a.txt", "sync-test:b.txt")
	if c, ok := remoteContent(t, client, "sync-test", "b.txt"); !ok || c != "alpha" {
		t.Errorf("renamed file: %q %v", c, ok)
	}
	if _, ok := remoteContent(t, client, "sync-test", "a.txt"); ok {
		t.Error("the old name remains")
	}

	// Into a folder keeps the name; by full name of the bucket.
	mustMv(false, "tester--sync-test:b.txt", "sync-test:docs/")
	if c, _ := remoteContent(t, client, "sync-test", "docs/b.txt"); c != "alpha" {
		t.Errorf("moved into a folder: %q", c)
	}

	// A folder moves whole, to a new name and then into an existing folder.
	mustMv(false, "sync-test:logs", "sync-test:archive")
	expectKeysWithPrefixIn(t, client, "sync-test", "archive/", "archive/app.log", "archive/old/one.log", "archive/old/two.tmp")
	expectNoneUnder(t, client, "sync-test", "logs/")
	mustMv(false, "sync-test:archive/old/", "sync-test:docs/")
	expectKeysWithPrefixIn(t, client, "sync-test", "docs/", "docs/b.txt", "docs/old/one.log", "docs/old/two.tmp")

	// An empty folder moves by its marker.
	mustMv(false, "sync-test:empty/", "sync-test:moved-empty")
	if got := bucketKeys(t, client, "sync-test"); !contains(got, "moved-empty/") || contains(got, "empty/") {
		t.Errorf("empty folder: %v", got)
	}

	// Several, and wildcards, go into a folder.
	putRemote(t, client, "sync-test", "x1.txt", "1")
	putRemote(t, client, "sync-test", "x2.txt", "2")
	mustMv(false, "sync-test:x1.txt", "sync-test:x2.txt", "sync-test:xs/")
	expectKeysWithPrefixIn(t, client, "sync-test", "xs/", "xs/x1.txt", "xs/x2.txt")
	mustMv(false, "sync-test:docs/old/*.log", "sync-test:logs-only/")
	expectKeysWithPrefixIn(t, client, "sync-test", "logs-only/", "logs-only/one.log")
	if _, ok := remoteContent(t, client, "sync-test", "docs/old/two.tmp"); !ok {
		t.Error("a file the wildcard did not match was moved")
	}

	// Refusals change nothing.
	fails("same", false, "sync-test:keep.txt", "sync-test:keep.txt")
	fails("into itself", false, "sync-test:archive", "sync-test:archive/inside")
	fails("no such file or folder", false, "sync-test:nothing", "sync-test:else")
	fails("whole bucket", false, "sync-test:", "other:x/")
	fails("not a folder", false, "sync-test:keep.txt", "sync-test:xs/x1.txt", "sync-test:newname")
	fails("bucket not found", false, "sync-test:keep.txt", "nobucket:x")
	putRemote(t, client, "sync-test", "target.txt", "target")
	fails("already exists", false, "sync-test:keep.txt", "sync-test:target.txt")
	if c, _ := remoteContent(t, client, "sync-test", "target.txt"); c != "target" {
		t.Errorf("a refused move changed the destination: %q", c)
	}
	mustMv(true, "sync-test:keep.txt", "sync-test:target.txt")
	if c, _ := remoteContent(t, client, "sync-test", "target.txt"); c != "keep" {
		t.Errorf("overwrite: %q", c)
	}

	// Between buckets: copied on the server, then removed.
	mustMv(false, "sync-test:archive", "other:from-sync/")
	expectKeysWithPrefixIn(t, client, "other", "from-sync/", "from-sync/archive/app.log")
	expectNoneUnder(t, client, "sync-test", "archive/")
	putRemote(t, client, "sync-test", "single.txt", "single")
	mustMv(false, "sync-test:single.txt", "other:renamed.txt")
	if c, _ := remoteContent(t, client, "other", "renamed.txt"); c != "single" {
		t.Errorf("moved to another bucket: %q", c)
	}
	if _, ok := remoteContent(t, client, "sync-test", "single.txt"); ok {
		t.Error("the source remains after a move between buckets")
	}
	putRemote(t, client, "sync-test", "clash.txt", "new")
	putRemote(t, client, "other", "clash.txt", "old")
	fails("already exists", false, "sync-test:clash.txt", "other:clash.txt")
	if _, ok := remoteContent(t, client, "sync-test", "clash.txt"); !ok {
		t.Error("a refused move between buckets removed the source")
	}
}

func expectNoneUnder(t *testing.T, client *apiclient.ApiClient, bucket, prefix string) {
	t.Helper()
	for _, k := range bucketKeys(t, client, bucket) {
		if strings.HasPrefix(k, prefix) {
			t.Errorf("%s still holds %s", bucket, k)
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
