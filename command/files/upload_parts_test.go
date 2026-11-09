package command_files

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

// partsSent counts the parts the test server received.
var partsSent atomic.Int64

// failPuts makes the test server refuse whole-file uploads while set, as a
// server restarting does.
var failPuts atomic.Bool

func smallParts(t *testing.T) {
	t.Helper()
	ps, pa := partSize, partedAt
	partSize, partedAt = 1024, 2048
	t.Cleanup(func() { partSize, partedAt, afterPart = ps, pa, nil })
}

func randomData(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

// A large file goes in parts; one stopped part way carries on, sending only
// the parts the server does not hold, when the sync runs again.
func TestUploadInPartsCarriesOn(t *testing.T) {
	smallParts(t)
	client, _ := newFilesServer(t)
	src := t.TempDir()
	data := randomData(t, 5000) // five parts
	os.WriteFile(filepath.Join(src, "big.bin"), data, 0644)

	// Stopped after the second part.
	ctx, cancel := context.WithCancel(context.Background())
	afterPart = func(n int) {
		if n == 2 {
			cancel()
		}
	}
	if err := syncUp(ctx, client, src, "sync-test", "parts/", false, false); err == nil {
		t.Fatal("the stopped upload succeeded")
	}
	if got, ok := remoteContent(t, client, "sync-test", "parts/big.bin"); ok {
		t.Fatalf("a file was made from part of the upload: %d bytes", len(got))
	}

	// Run again: the three parts not sent are sent, and the file is whole.
	afterPart = nil
	partsSent.Store(0)
	out := &logBuf{}
	if err := runEngine(context.Background(), client, syncOptions{mode: modeUp, dir: src, bucket: "sync-test", prefix: "parts/", log: out.logger()}, nil); err != nil {
		t.Fatal(err)
	}
	if n := partsSent.Load(); n != 3 {
		t.Errorf("carrying on sent %d parts, want 3", n)
	}
	if !logged(out.String(), "uploaded", `"content":"carried on`) {
		t.Errorf("the sync did not say it carried on:\n%s", out.String())
	}
	if got, _ := remoteContent(t, client, "sync-test", "parts/big.bin"); !bytes.Equal([]byte(got), data) {
		t.Errorf("the file sent in parts is %d bytes, not the %d sent", len(got), len(data))
	}
	if entries, _ := filepath.Glob(filepath.Join(mustSyncDir(t), "upload-*.json")); len(entries) != 0 {
		t.Errorf("upload records left: %v", entries)
	}
}

// A file changed since its upload stopped is sent afresh, whole.
func TestUploadInPartsChangedFileSentAfresh(t *testing.T) {
	smallParts(t)
	client, _ := newFilesServer(t)
	src := t.TempDir()
	path := filepath.Join(src, "big.bin")
	os.WriteFile(path, randomData(t, 5000), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	afterPart = func(n int) {
		if n == 2 {
			cancel()
		}
	}
	syncUp(ctx, client, src, "sync-test", "fresh/", false, false)

	afterPart = nil
	changed := randomData(t, 5000)
	os.WriteFile(path, changed, 0644)
	partsSent.Store(0)
	if err := syncUp(context.Background(), client, src, "sync-test", "fresh/", false, false); err != nil {
		t.Fatal(err)
	}
	if n := partsSent.Load(); n != 5 {
		t.Errorf("a changed file sent %d parts, want all 5", n)
	}
	if got, _ := remoteContent(t, client, "sync-test", "fresh/big.bin"); !bytes.Equal([]byte(got), changed) {
		t.Error("the file holds the old content")
	}
}

// An upload in parts is a conditional write as a whole one is: it fails
// when the file there is not the one expected.
func TestUploadInPartsConditional(t *testing.T) {
	smallParts(t)
	client, _ := newFilesServer(t)
	ctx := context.Background()
	if _, err := client.PutFileObject(ctx, "sync-test", "cond/big.bin", bytes.NewReader([]byte("there")), 5, "", apiclient.FileAttrs{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "big.bin")
	os.WriteFile(path, randomData(t, 3000), 0644)
	f, _ := os.Open(path)
	defer f.Close()
	info, _ := f.Stat()
	if _, _, err := uploadInParts(ctx, client, f, info, "sync-test", "cond/big.bin", "", apiclient.FileAttrs{}, "", true); !apiclient.IsPreconditionFailed(err) {
		t.Errorf("an upload if absent over a file: %v, want a failed precondition", err)
	}
	if _, _, err := uploadInParts(ctx, client, f, info, "sync-test", "cond/big.bin", "", apiclient.FileAttrs{}, "not-the-etag", false); !apiclient.IsPreconditionFailed(err) {
		t.Errorf("an upload if-match with another ETag: %v, want a failed precondition", err)
	}
	if got, _ := remoteContent(t, client, "sync-test", "cond/big.bin"); got != "there" {
		t.Errorf("the file was replaced: %d bytes", len(got))
	}
}

func mustSyncDir(t *testing.T) string {
	t.Helper()
	sd, err := syncDir()
	if err != nil {
		t.Fatal(err)
	}
	return sd
}
