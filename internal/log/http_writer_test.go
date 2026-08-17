package log

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func shortBackoffs(t *testing.T) {
	t.Helper()
	orig := flushBackoffs
	flushBackoffs = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { flushBackoffs = orig })
}

func newTestWriter(t *testing.T, url string) *httpWriter {
	t.Helper()
	return newHTTPWriter(url, "ndjson", "test", nil, "", "", "").(*httpWriter)
}

func TestFlushRetriesOnErrorThenSucceeds(t *testing.T) {
	shortBackoffs(t)

	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := newTestWriter(t, srv.URL)
	w.Write([]byte(`{"msg":"hello","time":"2026-08-15T10:00:00Z"}` + "\n"))
	w.flush()

	if got := requests.Load(); got != 3 {
		t.Fatalf("expected 3 attempts (2 failures + success), got %d", got)
	}
}

func TestFlushDropsAfterRetriesExhausted(t *testing.T) {
	shortBackoffs(t)

	var attempts atomic.Int32
	var spooled atomic.Int32
	orig := spoolBatch
	spoolBatch = func(lines [][]byte) { spooled.Add(int32(len(lines))) }
	t.Cleanup(func() { spoolBatch = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	w := newTestWriter(t, srv.URL)
	w.Write([]byte(`{"msg":"lost","time":"2026-08-15T10:00:00Z"}` + "\n"))
	w.flush()

	if got := attempts.Load(); got != int32(len(flushBackoffs)+1) {
		t.Fatalf("expected %d attempts, got %d", len(flushBackoffs)+1, got)
	}
	if spooled.Load() != 1 {
		t.Errorf("failed batch should be handed to spoolBatch, got %d lines", spooled.Load())
	}
}

func TestFlushNoRetryOnClientError(t *testing.T) {
	shortBackoffs(t)

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	w := newTestWriter(t, srv.URL)
	w.Write([]byte(`{"msg":"rejected","time":"2026-08-15T10:00:00Z"}` + "\n"))
	w.flush()

	if got := attempts.Load(); got != 1 {
		t.Fatalf("4xx should not be retried, got %d attempts", got)
	}
}

func TestStderrFailoverMirrorsAndRecovers(t *testing.T) {
	shortBackoffs(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	w := newTestWriter(t, srv.URL)
	stderr := &bytes.Buffer{}
	w.stderr = stderr

	// First failed batch: ERROR marker on the transition, then the records.
	w.Write([]byte(`{"msg":"during outage","time":"2026-08-16T10:00:00Z"}` + "\n"))
	w.flush()
	out := stderr.String()
	if !strings.Contains(out, "knot: ERROR: log output endpoint unreachable") {
		t.Errorf("expected degraded marker on stderr, got: %s", out)
	}
	if !strings.Contains(out, `"during outage"`) {
		t.Errorf("failed batch records should be mirrored to stderr, got: %s", out)
	}

	// While degraded, new records are teed to stderr as they arrive.
	stderr.Reset()
	w.Write([]byte(`{"msg":"teed live","time":"2026-08-16T10:00:01Z"}` + "\n"))
	if !strings.Contains(stderr.String(), `"teed live"`) {
		t.Errorf("degraded writer should tee records to stderr, got: %s", stderr.String())
	}

	// Recovery: one marker, then teeing stops.
	srv.Close()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	w.url = ok.URL

	stderr.Reset()
	w.Write([]byte(`{"msg":"after recovery","time":"2026-08-16T10:00:02Z"}` + "\n"))
	w.flush()
	if !strings.Contains(stderr.String(), "knot: log output recovered") {
		t.Errorf("expected recovery marker on stderr, got: %s", stderr.String())
	}

	stderr.Reset()
	w.Write([]byte(`{"msg":"healthy again","time":"2026-08-16T10:00:03Z"}` + "\n"))
	w.flush()
	if strings.Contains(stderr.String(), `"healthy again"`) {
		t.Error("recovered writer should stop teeing to stderr")
	}
}

func TestDegradedMarkerOnlyOnTransition(t *testing.T) {
	shortBackoffs(t)

	w := &httpWriter{url: "http://gone", stderr: &bytes.Buffer{}}

	// Two consecutive failed batches produce one marker, both batches mirrored.
	w.drop([][]byte{[]byte(`{"msg":"one"}`)})
	w.drop([][]byte{[]byte(`{"msg":"two"}`)})
	out := w.stderr.(*bytes.Buffer).String()
	if strings.Count(out, "knot: ERROR:") != 1 {
		t.Errorf("marker should appear once per outage, got: %s", out)
	}
	if !strings.Contains(out, `"one"`) || !strings.Contains(out, `"two"`) {
		t.Errorf("all failed records should be mirrored, got: %s", out)
	}
}

func TestRedactURL(t *testing.T) {
	cases := map[string]string{
		"http://user:secret@vl:9428/insert/jsonline": "http://user@vl:9428/insert/jsonline",
		"http://vl:9428/insert/jsonline":             "http://vl:9428/insert/jsonline",
	}
	for in, want := range cases {
		if got := redactURL(in); got != want {
			t.Errorf("redactURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatOrDefault(t *testing.T) {
	if got := formatOrDefault(""); got != "ndjson" {
		t.Errorf("empty format should default to ndjson, got %s", got)
	}
	if got := formatOrDefault("gelf"); got != "gelf" {
		t.Errorf("explicit format should pass through, got %s", got)
	}
}

func TestEncodeGELF(t *testing.T) {
	w := &httpWriter{format: "gelf", stream: "knot"}

	body, contentType := w.encode([][]byte{
		[]byte(`{"time":"2026-08-15T10:00:00Z","level":"ERROR","msg":"boom","stream":"audit","space_id":"s1"}` + "\n"),
		[]byte(`{"time":"2026-08-15T10:00:01Z","level":"INFO","msg":"ok"}` + "\n"),
	})

	if contentType != "application/json" {
		t.Errorf("unexpected content type %s", contentType)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 gelf messages, got %d", len(lines))
	}

	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["version"] != "1.1" || rec["short_message"] != "boom" {
		t.Errorf("unexpected gelf core fields: %v", rec)
	}
	if rec["level"] != float64(3) {
		t.Errorf("ERROR should map to gelf level 3, got %v", rec["level"])
	}
	if rec["host"] != "audit" {
		t.Errorf("stream field should become host, got %v", rec["host"])
	}
	if ts, ok := rec["timestamp"].(float64); !ok || ts < 1.7e9 {
		t.Errorf("timestamp should be unix seconds, got %v", rec["timestamp"])
	}
	if rec["_space_id"] != "s1" || rec["_stream"] != "audit" {
		t.Errorf("additional fields should be underscore-prefixed: %v", rec)
	}

	if err := json.Unmarshal([]byte(lines[1]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["level"] != float64(6) {
		t.Errorf("INFO should map to gelf level 6, got %v", rec["level"])
	}
	if rec["host"] != "knot" {
		t.Errorf("writer default stream should be host fallback, got %v", rec["host"])
	}
}

func TestGelfLevelMapping(t *testing.T) {
	cases := map[string]int{
		"PANIC": 1, "FATAL": 2, "ERROR": 3, "WARN": 4, "WARNING": 4,
		"INFO": 6, "DEBUG": 7, "TRACE": 7, "anything": 7,
	}
	for name, want := range cases {
		if got := gelfLevel(name); got != want {
			t.Errorf("%s: expected %d, got %d", name, want, got)
		}
	}
}

func TestGelfPostsOneMessagePerRequest(t *testing.T) {
	shortBackoffs(t)

	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(strings.TrimSpace(string(body)), "\n") {
			t.Errorf("gelf body must be a single message, got batch: %s", body)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	w := newTestWriter(t, srv.URL)
	w.format = "gelf"
	for i := 0; i < 3; i++ {
		w.Write([]byte(`{"msg":"one","time":"2026-08-17T10:00:0` + string(rune('0'+i)) + `Z","level":"INFO"}` + "\n"))
	}
	w.flush()

	if got := requests.Load(); got != 3 {
		t.Fatalf("expected 3 separate gelf posts, got %d", got)
	}
}

func TestGelfFlushFailureMirrorsToStderr(t *testing.T) {
	shortBackoffs(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	w := newTestWriter(t, srv.URL)
	w.format = "gelf"
	stderr := &bytes.Buffer{}
	w.stderr = stderr

	w.Write([]byte(`{"msg":"gelf one","time":"2026-08-17T10:00:00Z","level":"INFO"}` + "\n"))
	w.Write([]byte(`{"msg":"gelf two","time":"2026-08-17T10:00:01Z","level":"INFO"}` + "\n"))
	w.flush()

	out := stderr.String()
	if !strings.Contains(out, "knot: ERROR: log output endpoint unreachable") {
		t.Errorf("expected degraded marker on gelf flush failure, got: %s", out)
	}
	if !strings.Contains(out, "gelf one") || !strings.Contains(out, "gelf two") {
		t.Errorf("undelivered gelf messages should be mirrored to stderr, got: %s", out)
	}
	if strings.Contains(out, "recovered") {
		t.Errorf("a fully failed flush must not claim recovery, got: %s", out)
	}
}

func TestGelfPartialFailureDropsOnlyFailedMessages(t *testing.T) {
	shortBackoffs(t)

	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	w := newTestWriter(t, srv.URL)
	w.format = "gelf"
	stderr := &bytes.Buffer{}
	w.stderr = stderr

	w.Write([]byte(`{"msg":"delivered","time":"2026-08-17T10:00:00Z","level":"INFO"}` + "\n"))
	w.Write([]byte(`{"msg":"dropped","time":"2026-08-17T10:00:01Z","level":"INFO"}` + "\n"))
	w.flush()

	out := stderr.String()
	if !strings.Contains(out, "dropped") {
		t.Errorf("the failed message should be mirrored to stderr, got: %s", out)
	}
	if strings.Contains(out, "\"delivered\"") {
		t.Errorf("delivered messages must not be mirrored to stderr, got: %s", out)
	}
}
