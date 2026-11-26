package nfsserver

import (
	"errors"
	"io"
	"net"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/filestore"
)

// Close ends the connections, rather than leaving them to write to files
// it has already saved, and saves what was written and not yet committed.
func TestCloseEndsConnectionsAndSaves(t *testing.T) {
	store, err := filestore.Open(filestore.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	p := &filestore.Principal{UserId: "u1", Username: "alice", CanOwn: true}
	if _, err := store.CreateBucket(p, "alice--data"); err != nil {
		t.Fatal(err)
	}
	backend := &storeBackend{store: store, p: p}
	srv, err := Start(backend, Config{Listen: "127.0.0.1:0", StagingDir: t.TempDir(), FlushDelay: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(srv.Addr().String())
	n, _ := strconv.Atoi(port)
	tg, err := mountAt(t, n)
	for attempt := 1; err != nil && errors.Is(err, syscall.EADDRINUSE) && attempt < 10; attempt++ {
		tg, err = mountAt(t, n)
	}
	if err != nil {
		t.Fatal(err)
	}

	f, err := tg.OpenFile("data/staged.txt", 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("written, not committed")); err != nil {
		t.Fatal(err)
	}

	idle, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer idle.Close()
	time.Sleep(50 * time.Millisecond) // let the server take it

	closed := make(chan error, 1)
	go func() { closed <- srv.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return with a client connected")
	}

	// The connections were ended, not left open to a server that has
	// saved and gone.
	if err := idle.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if n, err := idle.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		t.Fatalf("idle connection after Close: read %d, %v; want it closed", n, err)
	}

	r, _, err := backend.Get(t.Context(), "data", "staged.txt", "")
	if err != nil {
		t.Fatalf("not saved at Close: %v", err)
	}
	defer r.Close()
	if got, _ := io.ReadAll(r); string(got) != "written, not committed" {
		t.Fatalf("saved %q", got)
	}
}
