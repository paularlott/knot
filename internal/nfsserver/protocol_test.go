package nfsserver

import (
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/filestore"
	nfsc "github.com/willscott/go-nfs-client/nfs"
	"github.com/willscott/go-nfs-client/nfs/rpc"
)

// mountTest serves a store over NFS and mounts it with an NFS client.
func mountTest(t *testing.T) (*nfsc.Target, *storeBackend) {
	t.Helper()
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
	t.Cleanup(func() { srv.Close() })

	_, port, _ := net.SplitHostPort(srv.Addr().String())
	n, _ := strconv.Atoi(port)
	client, err := nfsc.DialServiceAtPort("127.0.0.1", n)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	m := &nfsc.Mount{Client: client}
	target, err := m.Mount("/", rpc.AuthNull)
	if err != nil {
		t.Fatal(err)
	}
	return target, backend
}

// An open with O_CREAT of a file that exists, by a client without it cached,
// is an UNCHECKED create: it must not empty the file.
func TestCreateOfExistingFileKeepsIt(t *testing.T) {
	target, backend := mountTest(t)
	content := "precious content"
	if _, err := backend.Put(t.Context(), "data", "keep.txt", strings.NewReader(content), int64(len(content)), "", nil); err != nil {
		t.Fatal(err)
	}

	if _, err := target.Create("data/keep.txt", 0644); err != nil {
		t.Fatal(err)
	}

	f, err := target.Open("data/keep.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(f)
	if string(got) != content {
		t.Fatalf("after an unchecked create: %q, want %q", got, content)
	}
}

// Writing through the protocol, and reading back, end to end.
func TestProtocolWriteRead(t *testing.T) {
	target, _ := mountTest(t)
	f, err := target.OpenFile("data/new.txt", 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("written over NFS")); err != nil {
		t.Fatal(err)
	}
	f.Close()
	r, err := target.Open("data/new.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(r); string(got) != "written over NFS" {
		t.Fatalf("read back %q", got)
	}
}
