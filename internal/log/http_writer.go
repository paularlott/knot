package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// httpWriter is an io.Writer that forwards JSON log lines to an HTTP endpoint.
// It expects each Write call to contain one complete JSON object (as produced by
// the slog JSON handler).  Lines are batched and flushed every flushInterval or
// when the buffer reaches batchSize entries.
type httpWriter struct {
	url     string
	format  string
	stream  string
	headers map[string]string
	client  *http.Client

	mu       sync.Mutex
	buf      [][]byte
	stopCh   chan struct{}
	flushCh  chan struct{}
}

const (
	batchSize     = 100
	flushInterval = 2 * time.Second
)

func newHTTPWriter(rawURL, format, stream string, headers map[string]string) io.Writer {
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
				q.Set("_stream_fields", "stream")
			}
			u.RawQuery = q.Encode()
			rawURL = u.String()
		}
	}

	w := &httpWriter{
		url:     rawURL,
		format:  format,
		stream:  stream,
		headers: headers,
		client:  &http.Client{Timeout: 10 * time.Second},
		stopCh:  make(chan struct{}),
		flushCh: make(chan struct{}, 1),
	}
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
	w.mu.Unlock()

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

	body, contentType := w.encode(lines)
	if body == nil {
		return
	}

	req, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (w *httpWriter) encode(lines [][]byte) ([]byte, string) {
	switch w.format {
	case "loki":
		return w.encodeLoki(lines)
	case "elasticsearch":
		return w.encodeElasticsearch(lines)
	default: // ndjson
		return w.encodeNDJSON(lines)
	}
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
		// "stream" is declared as a _stream_field via query param
		if w.stream != "" {
			rec["stream"] = w.stream
		}
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

	stream := map[string]string{"job": w.stream}
	values := make([][2]string, 0, len(lines))
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
		b, _ := json.Marshal(rec)
		values = append(values, [2]string{ts, string(b)})
	}

	payload := lokiPayload{Streams: []lokiStream{{Stream: stream, Values: values}}}
	b, _ := json.Marshal(payload)
	return b, "application/json"
}

// encodeElasticsearch produces an ES bulk payload.
func (w *httpWriter) encodeElasticsearch(lines [][]byte) ([]byte, string) {
	index := w.stream
	if index == "" {
		index = "knot"
	}
	meta, _ := json.Marshal(map[string]any{"index": map[string]string{"_index": index}})

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
		if w.stream != "" {
			rec["stream"] = w.stream
		}
		b, _ := json.Marshal(rec)
		buf.Write(meta)
		buf.WriteByte('\n')
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), "application/x-ndjson"
}


