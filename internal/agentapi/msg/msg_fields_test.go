package msg

import (
	"testing"
	"time"

	"github.com/shamaton/msgpack/v3"
)

func encode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := msgpack.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func decode(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := msgpack.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func TestLogMessageFieldsRoundTrip(t *testing.T) {
	in := &LogMessage{
		Level:   LogLevelError,
		Service: "api",
		Message: "boom",
		Date:    time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC),
		Fields:  map[string]string{"request_id": "req-1", "status": "201"},
	}

	var out LogMessage
	decode(t, encode(t, in), &out)
	if out.Service != "api" || out.Level != LogLevelError || out.Message != "boom" {
		t.Errorf("core fields lost: %+v", out)
	}
	if out.Fields == nil || out.Fields["request_id"] != "req-1" || out.Fields["status"] != "201" {
		t.Errorf("structured fields lost: %v", out.Fields)
	}
}

// An old build (struct without Fields) must still decode a new payload —
// the extra key is ignored rather than rejected.
func TestLogMessageNewPayloadOldStruct(t *testing.T) {
	type legacyLogMessage struct {
		Level   LogLevel
		Service string
		Message string
		Date    time.Time
	}

	in := &LogMessage{
		Level: LogLevelInfo, Service: "s", Message: "m", Date: time.Now(),
		Fields: map[string]string{"k": "v"},
	}

	var legacy legacyLogMessage
	decode(t, encode(t, in), &legacy)
	if legacy.Service != "s" || legacy.Message != "m" {
		t.Errorf("legacy decode lost fields: %+v", legacy)
	}
}

// A new build must still decode an old payload — no fields key, nil result.
func TestLogMessageOldPayloadNewStruct(t *testing.T) {
	type legacyLogMessage struct {
		Level   LogLevel
		Service string
		Message string
		Date    time.Time
	}

	old := &legacyLogMessage{Level: LogLevelInfo, Service: "s", Message: "m", Date: time.Now()}

	var out LogMessage
	decode(t, encode(t, old), &out)
	if out.Fields != nil {
		t.Errorf("old payload should decode with nil fields, got %v", out.Fields)
	}
}

func TestMirrorLogEntryFieldsRoundTrip(t *testing.T) {
	in := &MirrorLogEntry{
		SpaceId: "sp1", SpaceName: "front", User: "alice",
		Service: "web", Level: LogLevelError, Message: "boom",
		Date:   time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC),
		Fields: map[string]string{"request_id": "req-2"},
	}

	var out MirrorLogEntry
	decode(t, encode(t, in), &out)
	if out.SpaceId != "sp1" || out.Service != "web" || out.Message != "boom" {
		t.Errorf("core fields lost: %+v", out)
	}
	if out.Fields == nil || out.Fields["request_id"] != "req-2" {
		t.Errorf("structured fields lost: %v", out.Fields)
	}
}

func TestRegisterNewPayloadOldStruct(t *testing.T) {
	type legacyRegister struct {
		SpaceId  string
		Version  string
		PeerPort uint16
	}

	// Register isn't marshalled as a pointer list like the log batches, but
	// the same msgpack tolerance applies; verify with a direct marshal.
	in := &Register{SpaceId: "s", Version: "v", LogSinkPort: 9428, LogSinkFormat: "vl"}

	var legacy legacyRegister
	decode(t, encode(t, in), &legacy)
	if legacy.SpaceId != "s" || legacy.Version != "v" {
		t.Errorf("legacy decode lost fields: %+v", legacy)
	}
}
