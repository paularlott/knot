package rest

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGzip(t *testing.T) {
	body := strings.Repeat(`{"key":"some/file.txt","size":12}`, 200)
	h := Gzip(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body[:100])
		w.(http.Flusher).Flush()
		io.WriteString(w, body[100:])
	})

	// Compressed for a client that asks.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "br, gzip;q=0.8")
	h(rec, req)
	if rec.Header().Get("Content-Encoding") != "gzip" || !rec.Flushed {
		t.Fatalf("headers %v flushed=%v", rec.Header(), rec.Flushed)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if string(got) != body {
		t.Errorf("decoded body differs")
	}
	if rec.Body.Len() >= len(body)/4 {
		t.Logf("compressed %d of %d", rec.Body.Len(), len(body))
	}

	// Plain for one that does not, or refuses it.
	for _, ae := range []string{"", "gzip;q=0", "identity"} {
		rec = httptest.NewRecorder()
		req = httptest.NewRequest("GET", "/", nil)
		if ae != "" {
			req.Header.Set("Accept-Encoding", ae)
		}
		h(rec, req)
		if rec.Header().Get("Content-Encoding") != "" || rec.Body.String() != body {
			t.Errorf("Accept-Encoding %q: encoded %q", ae, rec.Header().Get("Content-Encoding"))
		}
	}

	// No body is not wrapped.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	Gzip(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })(rec, req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Content-Encoding") != "" || rec.Body.Len() != 0 {
		t.Errorf("204 got %d %q %d", rec.Code, rec.Header().Get("Content-Encoding"), rec.Body.Len())
	}
}

func TestGunzipRequest(t *testing.T) {
	var buf strings.Builder
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte("hello records"))
	zw.Close()
	req := httptest.NewRequest("POST", "/", strings.NewReader(buf.String()))
	req.Header.Set("Content-Encoding", "gzip")
	if err := GunzipRequest(req); err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(req.Body)
	if string(got) != "hello records" {
		t.Errorf("got %q", got)
	}
}
