package nfsserver

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// A mkdir whose lookup began before another server made the folder, and
// finishes after that server's change was told of, must not record the
// folder as empty: what was made in it elsewhere would be known absent.
func TestMkdirRacingAChangeDoesNotHideEntries(t *testing.T) {
	fsys, _ := newTestFS(t)
	fsys.writer = "me"
	fsys.Live(true)
	b, err := fsys.bucket("data")
	if err != nil {
		t.Fatal(err)
	}
	sb := &slowBackend{Backend: fsys.backend, key: "own", asked: make(chan struct{}), release: make(chan struct{})}
	fsys.backend = sb

	done := make(chan error, 1)
	go func() { done <- fsys.MkdirAll("data/own", 0755) }()
	<-sb.asked
	// Meanwhile another server makes own/ and own/x/, and says so.
	for _, k := range []string{"own/", "own/x/"} {
		if _, err := sb.Backend.Put(context.Background(), "data", k, strings.NewReader(""), 0, "", nil); err != nil {
			t.Fatal(err)
		}
	}
	fsys.Changed([]string{b.Id}, []FileChanged{{BucketId: b.Id, Key: "own/", Writer: "other"}, {BucketId: b.Id, Key: "own/x/", Writer: "other"}})
	close(sb.release)
	if err := <-done; err != nil && !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}

	if _, err := fsys.Stat("data/own/x"); err != nil {
		t.Fatalf("a folder made elsewhere is hidden by the mkdir here: %v", err)
	}
	if fsys.knownAbsent("data", "own/x") {
		t.Fatal("known absent though it was made elsewhere")
	}
}

// busyStatBackend fails a lookup of one key once, as a server too busy to
// answer (a 503, a timeout).
type busyStatBackend struct {
	Backend
	key   string
	fails atomic.Int32
}

func (f *busyStatBackend) Stat(ctx context.Context, bucket, key string) (*Stat, error) {
	if key == f.key && f.fails.Add(-1) >= 0 {
		return nil, errors.New("unexpected status code: 503")
	}
	return f.Backend.Stat(ctx, bucket, key)
}

// A lookup the server could not answer for a moment is asked again, rather
// than the folder's handle being stale or its name not found.
func TestTransientLookupFailureIsRetried(t *testing.T) {
	fsys, _ := newTestFS(t)
	fsys.Live(true)
	if err := fsys.MkdirAll("data/own/B1-0", 0755); err != nil {
		t.Fatal(err)
	}
	h := newHandler(fsys)
	fh := h.ToHandle(fsys, []string{"data", "own", "B1-0"})
	fb := &busyStatBackend{Backend: fsys.backend, key: "own/B1-0"}
	fsys.backend = fb
	fsys.forget("data", "own/B1-0", false)
	fb.fails.Store(2)
	if _, _, err := h.FromHandle(fh); err != nil {
		t.Fatalf("handle stale over a lookup the server could not answer: %v", err)
	}
	fsys.forget("data", "own/B1-0", false)
	fb.fails.Store(2)
	if _, err := fsys.Stat("data/own/B1-0"); err != nil {
		t.Fatalf("not found over a lookup the server could not answer: %v", err)
	}
}
