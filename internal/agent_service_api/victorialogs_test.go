package agent_service_api

import (
	"bytes"
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
	sendLogMessage = func(service string, level msg.LogLevel, message string) {
		mu.Lock()
		defer mu.Unlock()
		*logs = append(*logs, capturedLog{service: service, level: level, message: message})
	}
	t.Cleanup(func() { sendLogMessage = orig })
	return logs, &mu
}

type capturedLog struct {
	service string
	level   msg.LogLevel
	message string
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
