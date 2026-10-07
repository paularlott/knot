//go:build integration

package suites

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/paularlott/knot/integration-tests/harness"
)

// TestLeafFilesStayApart proves file storage never crosses between a leaf
// node and its origin: a bucket and file made on the origin never reach the
// leaf, and one made on the leaf never reaches the origin, though users, tokens
// and templates replicate down as usual.
func TestLeafFilesStayApart(t *testing.T) {
	harness.Feature(t, "leaf-node")

	originFiles, leafFiles := t.TempDir(), t.TempDir()
	origin, err := harness.StartServer(cfg, bins, "fileorigin", "--allow-leaf-nodes", "--files-path", originFiles)
	if err != nil {
		t.Fatalf("boot origin: %v", err)
	}
	t.Cleanup(origin.Stop)
	originAdmin, err := harness.ProvisionAdmin(origin, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("provision origin admin: %v", err)
	}

	ctx, cancel := testCtx(60)
	defer cancel()
	originToken, code, err := originAdmin.Client.CreateToken(ctx, "leaf-token", nil)
	if err != nil {
		t.Fatalf("create origin token: %v (status %d)", err, code)
	}
	// Files on the origin before the leaf exists, and while it runs.
	if _, err := originAdmin.Client.CreateFileBucket(ctx, "origin-bucket"); err != nil {
		t.Fatalf("create origin bucket: %v", err)
	}
	data := []byte("origin data")
	if _, err := originAdmin.Client.PutFileObject(ctx, "origin-bucket", "o.txt", bytes.NewReader(data), int64(len(data)), "text/plain", time.Time{}); err != nil {
		t.Fatalf("put on origin: %v", err)
	}

	leaf, err := harness.StartServer(cfg, bins, "fileleaf", "--origin-server", origin.BaseURL, "--origin-token", originToken, "--files-path", leafFiles)
	if err != nil {
		t.Fatalf("boot leaf: %v", err)
	}
	t.Cleanup(leaf.Stop)
	leafAdmin, err := harness.LoginUser(leaf, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("login on the leaf with origin credentials: %v", err)
	}

	// The leaf has its own, empty, file storage.
	lctx, lcancel := testCtx(60)
	defer lcancel()
	if list, err := leafAdmin.Client.GetFileBuckets(lctx, true); err != nil || len(list.Buckets) != 0 {
		t.Fatalf("the leaf's buckets before it has made any: %+v %v", list, err)
	}
	if _, err := leafAdmin.Client.CreateFileBucket(lctx, "leaf-bucket"); err != nil {
		t.Fatalf("create leaf bucket: %v", err)
	}
	ldata := []byte("leaf data")
	if _, err := leafAdmin.Client.PutFileObject(lctx, "leaf-bucket", "l.txt", bytes.NewReader(ldata), int64(len(ldata)), "text/plain", time.Time{}); err != nil {
		t.Fatalf("put on leaf: %v", err)
	}
	// And one more on the origin once the leaf is up.
	if _, err := originAdmin.Client.PutFileObject(ctx, "origin-bucket", "o2.txt", bytes.NewReader(data), int64(len(data)), "text/plain", time.Time{}); err != nil {
		t.Fatalf("second put on origin: %v", err)
	}

	// Watch over a window, so a late transfer cannot slip past the first look.
	for i := 0; i < 8; i++ {
		time.Sleep(2 * time.Second)

		lb, err := leafAdmin.Client.GetFileBuckets(lctx, true)
		if err != nil || len(lb.Buckets) != 1 || lb.Buckets[0].Name != "admin--leaf-bucket" {
			t.Fatalf("the leaf's buckets: %+v %v", lb, err)
		}
		ob, err := originAdmin.Client.GetFileBuckets(ctx, true)
		if err != nil || len(ob.Buckets) != 1 || ob.Buckets[0].Name != "admin--origin-bucket" {
			t.Fatalf("the origin's buckets: %+v %v", ob, err)
		}
		if ol, err := originAdmin.Client.ListFileObjects(ctx, "admin--origin-bucket", "", "", "", 100); err != nil || len(ol.Objects) != 2 {
			t.Fatalf("origin files: %+v %v", ol, err)
		}
		if ll, err := leafAdmin.Client.ListFileObjects(lctx, "admin--leaf-bucket", "", "", "", 100); err != nil || len(ll.Objects) != 1 {
			t.Fatalf("leaf files: %+v %v", ll, err)
		}
		// Neither can read the other's files.
		if _, err := leafAdmin.Client.GetFileObject(lctx, "admin--origin-bucket", "o.txt"); err == nil {
			t.Fatal("the leaf read a file from the origin")
		}
		if _, err := originAdmin.Client.GetFileObject(ctx, "admin--leaf-bucket", "l.txt"); err == nil {
			t.Fatal("the origin read a file from the leaf")
		}
	}

	// Each server's content is its own, on disk.
	if n := countFiles(t, originFiles); n == 0 {
		t.Error("the origin stored nothing")
	}
	if entries, _ := os.ReadDir(leafFiles); len(entries) == 0 {
		t.Error("the leaf stored nothing")
	}
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}
