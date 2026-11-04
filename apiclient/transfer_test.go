package apiclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/paularlott/knot/build"
)

func withIdleTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := TransferIdleTimeout
	TransferIdleTimeout = d
	t.Cleanup(func() { TransferIdleTimeout = old })
}

// A download that stops moving data part way is given up on, with an error
// that says so, rather than waiting for TCP to notice.
func TestDownloadStalled(t *testing.T) {
	withIdleTimeout(t, 200*time.Millisecond)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(build.VersionHeader, build.Version)
		w.Header().Set("Content-Length", "1000")
		w.Write(bytes.Repeat([]byte("x"), 100))
		w.(http.Flusher).Flush()
		<-release // and nothing more
	}))
	defer server.Close()
	defer close(release) // before the server closes, which waits for its handlers
	c, err := NewClient(server.URL, "token", true)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	resp, err := c.GetFileObject(context.Background(), "b", "f")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !errors.Is(err, ErrTransferStalled) {
		t.Fatalf("a stalled download read: %v, want ErrTransferStalled", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("gave up after %s", d)
	}
}

// A download that keeps moving, however slowly and long in all, is not cut
// off.
func TestDownloadSlowButMoving(t *testing.T) {
	withIdleTimeout(t, 200*time.Millisecond)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(build.VersionHeader, build.Version)
		for i := 0; i < 10; i++ { // a second in all, each wait under the timeout
			w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			time.Sleep(100 * time.Millisecond)
		}
	}))
	defer server.Close()
	c, _ := NewClient(server.URL, "token", true)
	resp, err := c.GetFileObject(context.Background(), "b", "f")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || len(data) != 10 {
		t.Fatalf("a slow download: %d bytes, %v", len(data), err)
	}
}

// An upload the server stops taking in is given up on; once the whole body
// is sent, the server is waited for however long it takes to answer.
func TestUploadStalledAndServerWait(t *testing.T) {
	withIdleTimeout(t, 200*time.Millisecond)
	release := make(chan struct{})
	stall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // reads nothing
	}))
	defer stall.Close()
	defer close(release)
	c, _ := NewClient(stall.URL, "token", true)
	big := bytes.Repeat([]byte("x"), 64<<20) // more than the connection buffers hold
	_, err := c.PutFileObject(context.Background(), "b", "f", bytes.NewReader(big), int64(len(big)), "", FileAttrs{})
	if !errors.Is(err, ErrTransferStalled) {
		t.Fatalf("a stalled upload: %v, want ErrTransferStalled", err)
	}

	slowAnswer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		time.Sleep(600 * time.Millisecond) // saving: longer than the idle timeout
		w.Header().Set(build.VersionHeader, build.Version)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"key":"f","size":3}`))
	}))
	defer slowAnswer.Close()
	c, _ = NewClient(slowAnswer.URL, "token", true)
	if _, err := c.PutFileObject(context.Background(), "b", "f", bytes.NewReader([]byte("abc")), 3, "", FileAttrs{}); err != nil {
		t.Errorf("an upload the server took a while to answer: %v", err)
	}
}
