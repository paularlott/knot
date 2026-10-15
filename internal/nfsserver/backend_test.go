package nfsserver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
)

// rangeServer serves one file over the files API's object endpoint, with
// ranges as the real one does, counting the requests.
func rangeServer(t *testing.T, content []byte) (*apiBackend, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/files/objects/b/k" {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(content))
	}))
	t.Cleanup(srv.Close)
	client, err := apiclient.NewClient(srv.URL, "token", false)
	if err != nil {
		t.Fatal(err)
	}
	return NewAPIBackend(client).(*apiBackend), &requests
}

func TestAPIReadAtBlocks(t *testing.T) {
	content := make([]byte, 2*readBlock+12345)
	rand.New(rand.NewSource(1)).Read(content)
	b, requests := rangeServer(t, content)

	// Read it all in uneven pieces, as an NFS client would.
	var got []byte
	buf := make([]byte, 700000)
	for off := int64(0); ; {
		n, err := b.ReadAt(context.Background(), "b", "k", buf, off)
		got = append(got, buf[:n]...)
		off += int64(n)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatal("no progress before the end")
		}
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content differs: got %d bytes, want %d", len(got), len(content))
	}
	if n := requests.Load(); n > 4 {
		t.Fatalf("%d requests for 3 blocks", n)
	}

	// Exactly to the end is EOF; beyond it reads nothing.
	tail := make([]byte, 100)
	if n, err := b.ReadAt(context.Background(), "b", "k", tail, int64(len(content)-100)); n != 100 || !errors.Is(err, io.EOF) {
		t.Fatalf("read to the end: %d %v", n, err)
	}
	if n, err := b.ReadAt(context.Background(), "b", "k", tail, int64(len(content)+10)); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("read past the end: %d %v", n, err)
	}
	// A read across a block boundary.
	mid := make([]byte, 1000)
	if n, err := b.ReadAt(context.Background(), "b", "k", mid, readBlock-500); err != nil || !bytes.Equal(mid[:n], content[readBlock-500:readBlock+500]) {
		t.Fatalf("read across blocks: %d %v", n, err)
	}
}

func TestAPIReadAtSharesFetches(t *testing.T) {
	content := make([]byte, readBlock)
	b, requests := rangeServer(t, content)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := make([]byte, 4096)
			b.ReadAt(context.Background(), "b", "k", p, int64(i)*4096)
		}(i)
	}
	wg.Wait()
	if n := requests.Load(); n != 1 {
		t.Fatalf("%d requests for one block read at once", n)
	}

	// Changing the file drops what was kept of it.
	b.forgetBlocks("b", "k")
	b.ReadAt(context.Background(), "b", "k", make([]byte, 10), 0)
	if n := requests.Load(); n != 2 {
		t.Fatalf("block kept after the file changed: %d requests", n)
	}
}
