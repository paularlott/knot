package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// httpWriter is an io.Writer that forwards JSON log lines to an HTTP endpoint.
// It expects each Write call to contain one complete JSON object (as produced by
// the slog JSON handler).  Lines are batched and flushed every flushInterval or
// when the buffer reaches batchSize entries.
type httpWriter struct {
	url      string
	format   string
	stream   string
	headers  map[string]string
	username string // optional HTTP basic auth username
	password string // optional HTTP basic auth password
	token    string // optional bearer token (Authorization: Bearer <token>)
	client   *http.Client

	mu      sync.Mutex
	buf     [][]byte
	stopCh  chan struct{}
	flushCh chan struct{}

	// stderr is the local fallback: while the endpoint is unreachable the
	// records are mirrored here so they are never lost from view. A var on
	// the struct so tests can capture it.
	stderr   io.Writer
	degraded bool
}

const (
	batchSize     = 100
	flushInterval = 2 * time.Second
)

func newHTTPWriter(rawURL, format, stream string, headers map[string]string, username, password, token string) io.Writer {
	// Append VictoriaLogs field-mapping query params if not already present
	if format == "ndjson" || format == "" || format == "elasticsearch" {
		if u, err := url.Parse(rawURL); err == nil {
			q := u.Query()
			if q.Get("_msg_field") == "" {
				q.Set("_msg_field", "_msg")
			}
			if q.Get("_time_field") == "" {
				q.Set("_time_field", "_time")
			}
			if stream != "" && q.Get("_stream_fields") == "" {
				q.Set("_stream_fields", "source,service,level")
			}
			u.RawQuery = q.Encode()
			rawURL = u.String()
		}
	}

	w := &httpWriter{
		url:      rawURL,
		format:   format,
		stream:   stream,
		headers:  headers,
		username: username,
		password: password,
		token:    token,
		client:   &http.Client{Timeout: 10 * time.Second},
		stopCh:   make(chan struct{}),
		flushCh:  make(chan struct{}, 1),
		stderr:   os.Stderr,
	}
	fmt.Fprintf(w.stderr, "knot: started, sending logs to external service %s (format %s)\n", redactURL(rawURL), formatOrDefault(format))
	go w.run()
	return w
}

func (w *httpWriter) Write(p []byte) (int, error) {
	line := make([]byte, len(p))
	copy(line, p)
	// Strip trailing newline — we'll add separators per format
	line = bytes.TrimRight(line, "\n")
	if len(line) == 0 {
		return len(p), nil
	}

	w.mu.Lock()
	w.buf = append(w.buf, line)
	flush := len(w.buf) >= batchSize
	degraded := w.degraded
	w.mu.Unlock()

	// While the endpoint is unreachable, mirror records to stderr as they
	// arrive so the failure window is fully visible locally.
	if degraded {
		w.stderr.Write(append(line, '\n'))
	}

	if flush {
		select {
		case w.flushCh <- struct{}{}:
		default:
		}
	}
	return len(p), nil
}

func (w *httpWriter) run() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.flush()
		case <-w.flushCh:
			w.flush()
		case <-w.stopCh:
			w.flush()
			return
		}
	}
}

func (w *httpWriter) flush() {
	w.mu.Lock()
	if len(w.buf) == 0 {
		w.mu.Unlock()
		return
	}
	lines := w.buf
	w.buf = nil
	w.mu.Unlock()

	// GELF is posted one message per request: newline-batched GELF over
	// HTTP only works with Graylog's non-default "Enable Bulk Receiving"
	// option, so batching would silently fail on stock inputs.
	if w.format == "gelf" {
		var undelivered [][]byte
		for _, line := range lines {
			if body, contentType := w.encode([][]byte{line}); body != nil && w.post(body, contentType) {
				continue
			}
			undelivered = append(undelivered, line)
		}
		if len(undelivered) > 0 {
			// Same handling as a failed batch: stderr mirror and degraded
			// mode. Cleared below only once a flush delivers every message.
			// Delivery is at-least-once: a post that timed out after the
			// endpoint processed it still gets dropped to stderr, so
			// exactly-once consumers must deduplicate — GELF carries no
			// idempotency key to do it for them.
			w.drop(undelivered)
			return
		}
		w.mu.Lock()
		recovered := w.degraded
		w.degraded = false
		w.mu.Unlock()
		if recovered {
			fmt.Fprintf(w.stderr, "knot: log output recovered, sending logs to the external service only\n")
		}
		return
	}

	body, contentType := w.encode(lines)
	if body == nil {
		return
	}

	if w.post(body, contentType) {
		w.mu.Lock()
		recovered := w.degraded
		w.degraded = false
		w.mu.Unlock()
		if recovered {
			fmt.Fprintf(w.stderr, "knot: log output recovered, sending logs to the external service only\n")
		}
		return
	}
	w.drop(lines)
}

// flushBackoffs is the pause between delivery attempts for a batch. A var so
// tests can shorten it.
var flushBackoffs = []time.Duration{500 * time.Millisecond, 1 * time.Second, 2 * time.Second}

// post delivers the body, retrying with backoff on transport errors and
// retryable status codes (5xx, 429). A 4xx means the endpoint rejected the
// payload itself — retrying cannot help, so the batch is dropped. Returns
// true when the batch was delivered.
func (w *httpWriter) post(body []byte, contentType string) bool {
	for attempt := 0; ; attempt++ {
		delivered, retry := w.tryPost(body, contentType)
		if delivered {
			return true
		}
		if !retry || attempt >= len(flushBackoffs) {
			return false
		}
		time.Sleep(flushBackoffs[attempt])
	}
}

func (w *httpWriter) tryPost(body []byte, contentType string) (delivered, retry bool) {
	req, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return false, false
	}
	req.Header.Set("Content-Type", contentType)
	// A bearer token takes precedence over basic auth credentials; custom
	// headers are applied last so they can override either if required.
	if w.token != "" {
		req.Header.Set("Authorization", "Bearer "+w.token)
	} else if w.username != "" {
		req.SetBasicAuth(w.username, w.password)
	}
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return false, true
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, false
	}
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return false, true
	}
	return false, false
}

// spoolBatch is a hook for editions with an on-disk spool; the base
// implementation has nowhere to put failed batches.
var spoolBatch = func(lines [][]byte) {}

// drop handles a batch that could not be delivered after all retries: the
// records are mirrored to stderr (the configured destination is the endpoint
// that just failed) and the writer enters degraded mode, teeing every new
// record to stderr until a flush succeeds again. State markers are only
// written on transitions, so a flapping endpoint doesn't spam.
func (w *httpWriter) drop(lines [][]byte) {
	spoolBatch(lines)

	w.mu.Lock()
	already := w.degraded
	w.degraded = true
	w.mu.Unlock()

	if !already {
		fmt.Fprintf(w.stderr, "knot: ERROR: log output endpoint unreachable (%s), mirroring logs to stderr\n", redactURL(w.url))
	}
	for _, line := range lines {
		w.stderr.Write(append(line, '\n'))
	}
}

// redactURL strips any embedded credentials so markers can safely name the
// endpoint.
func redactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return rawURL
	}
	u.User = url.User(u.User.Username())
	return u.String()
}

func formatOrDefault(format string) string {
	if format == "" {
		return "ndjson"
	}
	return format
}

func (w *httpWriter) encode(lines [][]byte) ([]byte, string) {
	switch w.format {
	case "loki":
		return w.encodeLoki(lines)
	case "elasticsearch":
		return w.encodeElasticsearch(lines)
	case "gelf":
		return w.encodeGELF(lines)
	default: // ndjson
		return w.encodeNDJSON(lines)
	}
}

// streamFor returns the service value for a record: uses the record's own
// "service" field if present (the in-space service for space logs, "tunnel"
// for tunnel records), otherwise falls back to the writer default. The field
// name matches the agent's in-space ingest, so the same selector works
// against space logs, sinks and the external output.
func (w *httpWriter) streamFor(rec map[string]any) string {
	if s, ok := rec["service"].(string); ok && s != "" {
		return s
	}
	return w.stream
}

// encodeNDJSON produces newline-delimited JSON for VictoriaLogs / Vector.
func (w *httpWriter) encodeNDJSON(lines [][]byte) ([]byte, string) {
	var buf bytes.Buffer
	for _, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			rec = map[string]any{"_msg": string(line)}
		}
		// Map slog field names to VictoriaLogs expected names
		if msg, ok := rec["msg"]; ok {
			rec["_msg"] = msg
			delete(rec, "msg")
		}
		if t, ok := rec["time"]; ok {
			rec["_time"] = t
			delete(rec, "time")
		}
		// "service" is declared as a _stream_field via query param;
		// honour a per-record value, otherwise apply the writer default.
		rec["service"] = w.streamFor(rec)
		// Everything this writer ships came from a knot: one selector
		// (source:knot) sifts knot-delivered records from everything else a
		// shared logging service holds.
		rec["source"] = "knot"
		b, _ := json.Marshal(rec)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), "application/stream+json"
}

// encodeLoki produces a Loki push payload.
func (w *httpWriter) encodeLoki(lines [][]byte) ([]byte, string) {
	type lokiStream struct {
		Stream map[string]string `json:"stream"`
		Values [][2]string       `json:"values"`
	}
	type lokiPayload struct {
		Streams []lokiStream `json:"streams"`
	}

	// Group lines by stream label so each gets its own Loki stream entry.
	type group struct {
		stream map[string]string
		values [][2]string
	}
	groups := map[string]*group{}
	now := fmt.Sprintf("%d", time.Now().UnixNano())

	for _, line := range lines {
		var rec map[string]any
		ts := now
		if err := json.Unmarshal(line, &rec); err != nil {
			rec = map[string]any{"_msg": string(line)}
		}
		if t, ok := rec["time"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, t); err == nil {
				ts = fmt.Sprintf("%d", parsed.UnixNano())
			}
			delete(rec, "time")
		}
		if msg, ok := rec["msg"]; ok {
			rec["_msg"] = msg
			delete(rec, "msg")
		}
		streamLabel := w.streamFor(rec)
		delete(rec, "service")

		if _, ok := groups[streamLabel]; !ok {
			groups[streamLabel] = &group{stream: map[string]string{"job": streamLabel, "source": "knot"}}
		}
		b, _ := json.Marshal(rec)
		groups[streamLabel].values = append(groups[streamLabel].values, [2]string{ts, string(b)})
	}

	streams := make([]lokiStream, 0, len(groups))
	for _, g := range groups {
		streams = append(streams, lokiStream{Stream: g.stream, Values: g.values})
	}
	payload := lokiPayload{Streams: streams}
	b, _ := json.Marshal(payload)
	return b, "application/json"
}

// encodeElasticsearch produces an ES bulk payload.
func (w *httpWriter) encodeElasticsearch(lines [][]byte) ([]byte, string) {
	var buf bytes.Buffer
	for _, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			rec = map[string]any{"_msg": string(line)}
		}
		if msg, ok := rec["msg"]; ok {
			rec["_msg"] = msg
			delete(rec, "msg")
		}
		if t, ok := rec["time"]; ok {
			rec["_time"] = t
			delete(rec, "time")
		}
		index := w.streamFor(rec)
		if index == "" {
			index = "knot"
		}
		rec["service"] = index
		rec["source"] = "knot"
		meta, _ := json.Marshal(map[string]any{"index": map[string]string{"_index": index}})
		b, _ := json.Marshal(rec)
		buf.Write(meta)
		buf.WriteByte('\n')
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), "application/x-ndjson"
}

// encodeGELF produces newline-delimited GELF JSON messages (one per record)
// for Graylog's GELF HTTP input, which accepts multiple messages separated
// by newlines in a single request body.
func (w *httpWriter) encodeGELF(lines [][]byte) ([]byte, string) {
	var buf bytes.Buffer
	for _, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			rec = map[string]any{"msg": string(line)}
		}

		gelf := map[string]any{
			"version": "1.1",
			"host":    w.streamFor(rec),
			// Additional field (underscore per spec): one selector sifts
			// knot-delivered records in a shared Graylog.
			"_source": "knot",
		}
		if msg, ok := rec["msg"].(string); ok {
			gelf["short_message"] = msg
		} else {
			gelf["short_message"] = string(line)
		}
		if t, ok := rec["time"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, t); err == nil {
				gelf["timestamp"] = float64(parsed.UnixNano()) / 1e9
			}
		}
		if lvl, ok := rec["level"].(string); ok {
			gelf["level"] = gelfLevel(lvl)
		}

		// Everything else becomes a GELF additional field (underscore
		// prefix per spec), so stream / space_id / etc. survive the trip.
		for k, v := range rec {
			switch k {
			case "msg", "time", "level", "service":
				continue
			}
			if s, ok := v.(string); ok {
				gelf["_"+k] = s
			} else {
				gelf["_"+k] = v
			}
		}
		if s, ok := rec["service"].(string); ok && s != "" {
			gelf["_service"] = s
		}

		b, _ := json.Marshal(gelf)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), "application/json"
}

// gelfLevel maps slog level names to syslog severities (0 emerg – 7 debug).
func gelfLevel(level string) int {
	switch strings.ToUpper(level) {
	case "PANIC":
		return 1
	case "FATAL":
		return 2
	case "ERROR":
		return 3
	case "WARN", "WARNING":
		return 4
	case "INFO":
		return 6
	default: // DEBUG, TRACE
		return 7
	}
}
