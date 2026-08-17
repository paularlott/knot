package agent_service_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

func captureLogs(t *testing.T) (*[]capturedLog, *sync.Mutex) {
	t.Helper()
	var mu sync.Mutex
	logs := &[]capturedLog{}
	orig := sendLogMessage
	sendLogMessage = func(service string, level msg.LogLevel, message string, fields map[string]string) {
		mu.Lock()
		defer mu.Unlock()
		*logs = append(*logs, capturedLog{service: service, level: level, message: message, fields: fields})
	}
	t.Cleanup(func() { sendLogMessage = orig })
	return logs, &mu
}

type capturedLog struct {
	service string
	level   msg.LogLevel
	message string
	fields  map[string]string
}

func postJSONLine(t *testing.T, query, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/insert/jsonline"+query, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handleVictoriaLogs(rec, req)
	return rec
}

func TestVictoriaLogsDefaults(t *testing.T) {
	logs, mu := captureLogs(t)

	rec := postJSONLine(t, "",
		`{"_msg":"hello world","_time":"2026-08-15T10:00:00Z"}`+"\n"+
			`{"_msg":"second line"}`+"\n")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*logs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(*logs))
	}
	if (*logs)[0].message != "hello world" || (*logs)[0].service != "victorialogs" || (*logs)[0].level != msg.LogLevelInfo {
		t.Errorf("unexpected first message: %+v", (*logs)[0])
	}
}

func TestVictoriaLogsCustomFieldNames(t *testing.T) {
	logs, mu := captureLogs(t)

	rec := postJSONLine(t, "?_msg_field=message&_stream_fields=app,host",
		`{"message":"custom fields","app":"web","host":"box1"}`+"\n")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*logs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(*logs))
	}
	if (*logs)[0].message != "custom fields" {
		t.Errorf("message should come from _msg_field, got %q", (*logs)[0].message)
	}
	if (*logs)[0].service != "web" {
		t.Errorf("service should come from first _stream_fields entry, got %q", (*logs)[0].service)
	}
}

func TestVictoriaLogsServiceFieldWins(t *testing.T) {
	logs, mu := captureLogs(t)

	postJSONLine(t, "?_stream_fields=app",
		`{"_msg":"m","app":"app-name","service":"svc-name"}`+"\n")

	mu.Lock()
	defer mu.Unlock()
	if (*logs)[0].service != "svc-name" {
		t.Errorf("explicit service field should win, got %q", (*logs)[0].service)
	}
}

func TestVictoriaLogsLevels(t *testing.T) {
	logs, mu := captureLogs(t)

	postJSONLine(t, "",
		`{"_msg":"d","level":"debug"}`+"\n"+
			`{"_msg":"e","level":"error"}`+"\n"+
			`{"_msg":"n","level":3}`+"\n"+
			`{"_msg":"u"}`+"\n")

	mu.Lock()
	defer mu.Unlock()
	want := []msg.LogLevel{msg.LogLevelDebug, msg.LogLevelError, msg.LogLevelInfo, msg.LogLevelInfo}
	if len(*logs) != len(want) {
		t.Fatalf("expected %d messages, got %d", len(want), len(*logs))
	}
	for i, lvl := range want {
		if (*logs)[i].level != lvl {
			t.Errorf("message %d: expected level %d, got %d", i, lvl, (*logs)[i].level)
		}
	}
}

func TestVictoriaLogsInvalidJSON(t *testing.T) {
	captureLogs(t)
	rec := postJSONLine(t, "", "{not json}\n")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json, got %d", rec.Code)
	}
}

func TestVictoriaLogsBlankLines(t *testing.T) {
	logs, mu := captureLogs(t)
	rec := postJSONLine(t, "", "\n\n{\"_msg\":\"only\"}\n\n")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*logs) != 1 {
		t.Fatalf("blank lines should be skipped, got %d messages", len(*logs))
	}
}

func TestVictoriaLogsExtraFieldsCollected(t *testing.T) {
	logs, mu := captureLogs(t)

	postJSONLine(t, "?_stream_fields=service",
		`{"_msg":"m","service":"svc","request_id":"req-1","status":201,"ok":true}`+"\n")

	mu.Lock()
	defer mu.Unlock()
	if len(*logs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(*logs))
	}
	f := (*logs)[0].fields
	if f == nil {
		t.Fatal("structured fields should be collected")
	}
	if f["request_id"] != "req-1" || f["status"] != "201" || f["ok"] != "true" {
		t.Errorf("unexpected fields: %v", f)
	}
	if _, present := f["service"]; present {
		t.Errorf("service should be mapped, not carried as a field: %v", f)
	}
}

func TestGelfAdditionalFieldsCollected(t *testing.T) {
	logs, mu := captureLogs(t)

	body := `{"version":"1.1","host":"space-a","short_message":"boom","level":3,"facility":"web","_request_id":"req-9","_duration_ms":87}`
	req := httptest.NewRequest(http.MethodPost, "/gelf", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handleGelf(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*logs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(*logs))
	}
	if (*logs)[0].service != "web" || (*logs)[0].level != msg.LogLevelError || (*logs)[0].message != "boom" {
		t.Errorf("unexpected mapping: %+v", (*logs)[0])
	}
	f := (*logs)[0].fields
	if f == nil || f["request_id"] != "req-9" || f["duration_ms"] != "87" {
		t.Errorf("additional fields should be collected without the underscore: %v", f)
	}
}

func TestLokiJSONLineFieldsCollected(t *testing.T) {
	logs, mu := captureLogs(t)

	line, _ := json.Marshal(map[string]any{"msg": "request completed", "status": 201.0, "request_id": "req-2"})
	push := map[string]any{"streams": []map[string]any{{
		"stream": map[string]string{"label": "api"},
		"values": [][]string{{"1700000000", string(line)}},
	}}}
	body, _ := json.Marshal(push)
	req := httptest.NewRequest(http.MethodPost, "/loki/api/v1/push", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handleLoki(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*logs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(*logs))
	}
	if (*logs)[0].service != "api" || (*logs)[0].message != "request completed" {
		t.Errorf("unexpected mapping: %+v", (*logs)[0])
	}
	f := (*logs)[0].fields
	if f == nil || f["request_id"] != "req-2" || f["status"] != "201" {
		t.Errorf("JSON line fields should be collected: %v", f)
	}
}

func TestLokiPlainLineUnchanged(t *testing.T) {
	logs, mu := captureLogs(t)

	push := map[string]any{"streams": []map[string]any{{
		"stream": map[string]string{"label": "api"},
		"values": [][]string{{"1700000000", "plain old line"}},
	}}}
	body, _ := json.Marshal(push)
	req := httptest.NewRequest(http.MethodPost, "/loki/api/v1/push", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handleLoki(rec, req)

	mu.Lock()
	defer mu.Unlock()
	if len(*logs) != 1 || (*logs)[0].message != "plain old line" || (*logs)[0].fields != nil {
		t.Errorf("plain lines should pass through verbatim: %+v", *logs)
	}
}

func postNative(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/logs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handleLogMessage(rec, req)
	return rec
}

func TestNativeStructuredFieldsCollected(t *testing.T) {
	logs, mu := captureLogs(t)

	rec := postNative(t, `{"service":"api","level":"info","message":"done","request_id":"req-5","status":201}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*logs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(*logs))
	}
	l := (*logs)[0]
	if l.service != "api" || l.level != msg.LogLevelInfo || l.message != "done" {
		t.Errorf("unexpected mapping: %+v", l)
	}
	if l.fields == nil || l.fields["request_id"] != "req-5" || l.fields["status"] != "201" {
		t.Errorf("extra keys should be collected as fields: %v", l.fields)
	}
}

func TestNativeBackwardCompatible(t *testing.T) {
	logs, mu := captureLogs(t)

	rec := postNative(t, `{"service":"web","level":"error","message":"legacy sender"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("legacy 3-key payloads must still be accepted, got %d", rec.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*logs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(*logs))
	}
	l := (*logs)[0]
	if l.service != "web" || l.level != msg.LogLevelError || l.message != "legacy sender" || l.fields != nil {
		t.Errorf("legacy payload should map identically with no fields: %+v", l)
	}
}

func TestNativeInvalidLevelStillRejected(t *testing.T) {
	captureLogs(t)
	rec := postNative(t, `{"service":"web","level":"shouty","message":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid level must still 400, got %d", rec.Code)
	}
}
