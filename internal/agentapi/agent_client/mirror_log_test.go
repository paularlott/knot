package agent_client

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

type capturedRequest struct {
	path        string
	contentType string
	auth        string
	body        string
}

func captureServer(t *testing.T) (*httptest.Server, *[]capturedRequest, *sync.Mutex) {
	t.Helper()
	var mu sync.Mutex
	requests := &[]capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		uri := r.URL.Path
		if r.URL.RawQuery != "" {
			uri += "?" + r.URL.RawQuery
		}
		*requests = append(*requests, capturedRequest{uri, r.Header.Get("Content-Type"), r.Header.Get("Authorization"), string(body)})
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, requests, &mu
}

func sinkClient(t *testing.T, format string) *AgentClient {
	t.Helper()
	t.Setenv("KNOT_LOG_SINK_PORT", "9428")
	if format != "" {
		t.Setenv("KNOT_LOG_SINK_FORMAT", format)
	} else {
		t.Setenv("KNOT_LOG_SINK_FORMAT", "")
	}
	return NewAgentClient("server:3010", "sink-space")
}

func mirrorBatch() *msg.MirrorLogMessage {
	return &msg.MirrorLogMessage{Entries: []*msg.MirrorLogEntry{
		{SpaceId: "space-a", SpaceName: "frontend", User: "alice", Service: "web", Level: msg.LogLevelError, Message: "boom", Date: time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC), Fields: map[string]string{"request_id": "req-1", "status": "201"}},
		{SpaceId: "space-b", SpaceName: "backend", User: "alice", Service: "api", Level: msg.LogLevelInfo, Message: "ok", Date: time.Date(2026, 8, 15, 10, 0, 1, 0, time.UTC)},
	}}
}

func pointMirrorAt(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := mirrorTarget
	mirrorTarget = func(port int) string { return srv.URL }
	t.Cleanup(func() { mirrorTarget = orig })
}

func TestMirrorLogVLFormat(t *testing.T) {
	srv, requests, mu := captureServer(t)
	pointMirrorAt(t, srv)

	handleMirrorLog(sinkClient(t, ""), mirrorBatch())

	mu.Lock()
	defer mu.Unlock()
	if len(*requests) != 1 {
		t.Fatalf("expected one batch request, got %d", len(*requests))
	}
	req := (*requests)[0]
	if req.path != "/insert/jsonline?_stream_fields=service,level" {
		t.Errorf("expected VL insert with stream fields, got %s", req.path)
	}
	lines := strings.Split(strings.TrimSpace(req.body), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 jsonline records, got %d", len(lines))
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["_msg"] != "boom" || rec["service"] != "web" || rec["level"] != "ERROR" {
		t.Errorf("unexpected record: %v", rec)
	}
	if rec["actor"] != "alice" {
		t.Errorf("record should carry the owner as actor: %v", rec)
	}
	props, ok := rec["properties"].(map[string]any)
	if !ok || props["space_id"] != "space-a" || props["space_name"] != "frontend" {
		t.Errorf("space identity should be nested in properties: %v", rec)
	}
	if rec["request_id"] != "req-1" || rec["status"] != "201" {
		t.Errorf("structured fields should be flattened into the record: %v", rec)
	}
	if rec["_time"] != "2026-08-15T10:00:00Z" {
		t.Errorf("unexpected _time: %v", rec["_time"])
	}
}

func TestMirrorLogLokiFormat(t *testing.T) {
	srv, requests, mu := captureServer(t)
	pointMirrorAt(t, srv)

	handleMirrorLog(sinkClient(t, "loki"), mirrorBatch())

	mu.Lock()
	defer mu.Unlock()
	if len(*requests) != 1 {
		t.Fatalf("expected one push request, got %d", len(*requests))
	}
	req := (*requests)[0]
	if req.path != "/loki/api/v1/push" {
		t.Errorf("expected /loki/api/v1/push, got %s", req.path)
	}
	var payload struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][2]string       `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal([]byte(req.body), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Streams) != 2 {
		t.Fatalf("expected one stream per source space, got %d", len(payload.Streams))
	}
	// Stream order follows Go map iteration — look the space up, don't index.
	var spaceA *struct {
		Stream map[string]string `json:"stream"`
		Values [][2]string       `json:"values"`
	}
	for i := range payload.Streams {
		if payload.Streams[i].Stream["space"] == "space-a" {
			spaceA = &payload.Streams[i]
		}
	}
	if spaceA == nil {
		t.Fatalf("no stream for space-a: %v", payload.Streams)
	}
	if spaceA.Stream["space_name"] != "frontend" || spaceA.Stream["user"] != "alice" {
		t.Errorf("stream should carry space name and owner: %v", spaceA.Stream)
	}
}

func TestMirrorLogGelfFormat(t *testing.T) {
	srv, requests, mu := captureServer(t)
	pointMirrorAt(t, srv)

	handleMirrorLog(sinkClient(t, "gelf"), mirrorBatch())

	mu.Lock()
	defer mu.Unlock()
	if len(*requests) != 2 {
		t.Fatalf("GELF is per-message, expected 2 requests, got %d", len(*requests))
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte((*requests)[0].body), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["short_message"] != "boom" || rec["host"] != "space-a" || rec["facility"] != "web" {
		t.Errorf("unexpected gelf record: %v", rec)
	}
	if rec["_space_name"] != "frontend" || rec["_user"] != "alice" {
		t.Errorf("gelf record should carry space name and owner: %v", rec)
	}
	if rec["level"] != float64(3) {
		t.Errorf("error level should map to gelf 3, got %v", rec["level"])
	}
}

func TestMirrorLogJSONFormat(t *testing.T) {
	srv, requests, mu := captureServer(t)
	pointMirrorAt(t, srv)

	handleMirrorLog(sinkClient(t, "json"), mirrorBatch())

	mu.Lock()
	defer mu.Unlock()
	if len(*requests) != 2 {
		t.Fatalf("native format is per-message, expected 2 requests, got %d", len(*requests))
	}
	if (*requests)[0].path != "/logs" {
		t.Errorf("expected /logs, got %s", (*requests)[0].path)
	}
	var rec map[string]string
	if err := json.Unmarshal([]byte((*requests)[0].body), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["service"] != "web" || rec["level"] != "ERROR" || rec["message"] != "boom" {
		t.Errorf("unexpected record: %v", rec)
	}
	if rec["space_id"] != "space-a" || rec["space_name"] != "frontend" || rec["user"] != "alice" {
		t.Errorf("record should carry the source space and user: %v", rec)
	}
	if rec["request_id"] != "req-1" || rec["status"] != "201" {
		t.Errorf("record should carry structured fields: %v", rec)
	}
}

func TestMirrorLogLokiSplitsServicesPerStream(t *testing.T) {
	srv, requests, mu := captureServer(t)
	pointMirrorAt(t, srv)

	handleMirrorLog(sinkClient(t, "loki"), &msg.MirrorLogMessage{Entries: []*msg.MirrorLogEntry{
		{SpaceId: "space-a", SpaceName: "frontend", User: "alice", Service: "web", Level: msg.LogLevelInfo, Message: "from web", Date: time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)},
		{SpaceId: "space-a", SpaceName: "frontend", User: "alice", Service: "worker", Level: msg.LogLevelInfo, Message: "from worker", Date: time.Date(2026, 8, 15, 10, 0, 1, 0, time.UTC)},
	}})

	mu.Lock()
	defer mu.Unlock()
	var payload struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][2]string       `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal([]byte((*requests)[0].body), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Streams) != 2 {
		t.Fatalf("one space with two services must yield two streams, got %d: %v", len(payload.Streams), payload.Streams)
	}
	streams := map[string]string{}
	for _, s := range payload.Streams {
		if s.Stream["space"] != "space-a" {
			t.Fatalf("both streams belong to space-a, got %v", s.Stream)
		}
		streams[s.Stream["service"]] = s.Values[0][1]
	}
	if len(streams) != 2 {
		t.Fatalf("expected web and worker streams, got %v", streams)
	}
	if !strings.Contains(streams["web"], "from web") || !strings.Contains(streams["worker"], "from worker") {
		t.Errorf("each stream's entries must be in its own stream, got %v", streams)
	}
}

func TestMirrorLogRetriesThenSucceeds(t *testing.T) {
	orig := mirrorPostAttempts
	mirrorPostAttempts = []time.Duration{time.Millisecond}
	t.Cleanup(func() { mirrorPostAttempts = orig })

	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	pointMirrorAt(t, srv)

	handleMirrorLog(sinkClient(t, ""), mirrorBatch())

	if attempts != 2 {
		t.Errorf("expected retry after 503, got %d attempts", attempts)
	}
}

func TestMirrorLogNoSinkPort(t *testing.T) {
	srv, requests, mu := captureServer(t)
	pointMirrorAt(t, srv)

	client := sinkClient(t, "")
	client.logSinkPort = 0
	handleMirrorLog(client, mirrorBatch())

	mu.Lock()
	defer mu.Unlock()
	if len(*requests) != 0 {
		t.Errorf("no requests expected when the space is not a sink")
	}
}

func TestDefaultLogSinkFormat(t *testing.T) {
	cases := map[string]string{
		"":             "vl",
		"vl":           "vl",
		"VictoriaLogs": "vl",
		"victorialogs": "vl",
		"loki":         "loki",
		"gelf":         "gelf",
		"json":         "json",
		"nonsense":     "vl",
	}
	for env, want := range cases {
		t.Setenv("KNOT_LOG_SINK_FORMAT", env)
		if got := defaultLogSinkFormat(); got != want {
			t.Errorf("env %q: expected format %q, got %q", env, want, got)
		}
	}
}

func TestMirrorLogAuthBearer(t *testing.T) {
	srv, requests, mu := captureServer(t)
	pointMirrorAt(t, srv)

	client := sinkClient(t, "")
	t.Setenv("KNOT_LOG_SINK_TOKEN", "sekrit")
	client.logSinkToken = "sekrit" // env is read at construction; set directly too
	handleMirrorLog(client, mirrorBatch())

	mu.Lock()
	defer mu.Unlock()
	if len(*requests) == 0 || (*requests)[0].auth != "Bearer sekrit" {
		t.Errorf("expected bearer auth header, got %+v", *requests)
	}
}

func TestMirrorLogAuthBasic(t *testing.T) {
	srv, requests, mu := captureServer(t)
	pointMirrorAt(t, srv)

	client := sinkClient(t, "loki")
	client.logSinkUsername, client.logSinkPassword = "tenant", "tok123"
	handleMirrorLog(client, mirrorBatch())

	mu.Lock()
	defer mu.Unlock()
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("tenant:tok123"))
	if len(*requests) == 0 || (*requests)[0].auth != want {
		t.Errorf("expected basic auth header %q, got %+v", want, *requests)
	}
}

func TestMirrorLogTokenTakesPrecedence(t *testing.T) {
	srv, requests, mu := captureServer(t)
	pointMirrorAt(t, srv)

	client := sinkClient(t, "")
	client.logSinkToken = "tok"
	client.logSinkUsername, client.logSinkPassword = "u", "p"
	handleMirrorLog(client, mirrorBatch())

	mu.Lock()
	defer mu.Unlock()
	if len(*requests) == 0 || (*requests)[0].auth != "Bearer tok" {
		t.Errorf("token should take precedence over basic auth, got %+v", *requests)
	}
}

func TestMirrorMetadataNotOverwrittenByFields(t *testing.T) {
	// An app field that collides with knot's origin keys must not
	// misattribute the record at the sink.
	entry := &msg.MirrorLogEntry{
		SpaceId: "space-a", SpaceName: "frontend", User: "alice",
		Service: "web", Level: msg.LogLevelError, Message: "boom",
		Date:   time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC),
		Fields: map[string]string{"service": "evil", "user": "mallory", "level": "debug", "space_id": "space-x", "request_id": "req-9"},
	}
	batch := &msg.MirrorLogMessage{Entries: []*msg.MirrorLogEntry{entry}}

	// VL: metadata keys survive, unrelated fields pass through.
	vl := string(encodeMirrorVL(batch))
	for _, want := range []string{`"service":"web"`, `"actor":"alice"`, `"request_id":"req-9"`} {
		if !strings.Contains(vl, want) {
			t.Errorf("VL record missing %s: %s", want, vl)
		}
	}
	if strings.Contains(vl, `"service":"evil"`) || strings.Contains(vl, `"actor":"mallory"`) {
		t.Errorf("VL metadata was overwritten by app fields: %s", vl)
	}

	// Loki: msg and level in the line JSON stay knot's. Origin lives in the
	// stream labels, so app fields named service/user may appear in the line
	// without misattributing anything.
	var payload struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][2]string       `json:"values"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(encodeMirrorLoki(batch), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Streams) != 1 {
		t.Fatalf("expected one stream, got %d", len(payload.Streams))
	}
	if payload.Streams[0].Stream["service"] != "web" || payload.Streams[0].Stream["user"] != "alice" {
		t.Errorf("Loki stream labels lost origin: %v", payload.Streams[0].Stream)
	}
	var line map[string]any
	if err := json.Unmarshal([]byte(payload.Streams[0].Values[0][1]), &line); err != nil {
		t.Fatal(err)
	}
	if line["msg"] != "boom" || line["level"] != "ERROR" {
		t.Errorf("Loki line msg/level were overwritten: %v", line)
	}

	// JSON native: same rule, through the real delivery path.

	// GELF: app fields are underscore-prefixed, so nothing to clobber.
	srv, requests, mu := captureServer(t)
	pointMirrorAt(t, srv)
	handleMirrorLog(sinkClient(t, "json"), batch)
	mu.Lock()
	defer mu.Unlock()
	if len(*requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(*requests))
	}
	var native map[string]string
	if err := json.Unmarshal([]byte((*requests)[0].body), &native); err != nil {
		t.Fatal(err)
	}
	if native["service"] != "web" || native["user"] != "alice" || native["space_id"] != "space-a" || native["level"] != "ERROR" {
		t.Errorf("native metadata overwritten: %v", native)
	}
	if native["request_id"] != "req-9" {
		t.Errorf("unrelated field should pass through: %v", native)
	}
}
