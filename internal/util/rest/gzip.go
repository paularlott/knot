package rest

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

var gzipPool = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
	return w
}}

// AcceptsGzip reports whether the request accepts a gzip-encoded response.
func AcceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		enc, q, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(enc), "gzip") && strings.TrimSpace(q) != "q=0" {
			return true
		}
	}
	return false
}

// Gzip compresses the response of next for clients that accept it. It is for
// responses of records (listings, change feeds, backups) that are large and
// compress well; file content is sent as stored, so its checksums, ranges and
// ETags hold. A handler that flushes still streams.
func Gzip(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if !AcceptsGzip(r) || r.Method == http.MethodHead {
			next(w, r)
			return
		}
		gz := gzipPool.Get().(*gzip.Writer)
		gz.Reset(w)
		gw := &gzipWriter{ResponseWriter: w, gz: gz}
		defer func() {
			if gw.started {
				gz.Close()
			}
			gzipPool.Put(gz)
		}()
		next(gw, r)
	}
}

type gzipWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer
	started bool
	plain   bool // the response is not compressed (an empty or error body)
}

func (g *gzipWriter) WriteHeader(status int) {
	if g.started || g.plain {
		return
	}
	if status == http.StatusNoContent || status == http.StatusNotModified || status < 200 {
		g.plain = true
		g.ResponseWriter.WriteHeader(status)
		return
	}
	g.started = true
	h := g.Header()
	h.Del("Content-Length")
	h.Set("Content-Encoding", "gzip")
	g.ResponseWriter.WriteHeader(status)
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	if !g.started && !g.plain {
		g.WriteHeader(http.StatusOK)
	}
	if g.plain {
		return g.ResponseWriter.Write(b)
	}
	return g.gz.Write(b)
}

func (g *gzipWriter) Flush() {
	if g.started {
		g.gz.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the connection, for deadlines.
func (g *gzipWriter) Unwrap() http.ResponseWriter { return g.ResponseWriter }

// GunzipRequest decodes a request body sent with Content-Encoding: gzip.
func GunzipRequest(r *http.Request) error {
	if !strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		return nil
	}
	zr, err := gzip.NewReader(r.Body)
	if err != nil {
		return err
	}
	body := r.Body
	r.Body = struct {
		io.Reader
		io.Closer
	}{zr, closerFunc(func() error { zr.Close(); return body.Close() })}
	r.Header.Del("Content-Encoding")
	r.ContentLength = -1
	return nil
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }
