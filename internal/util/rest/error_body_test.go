package rest

import (
	"testing"

	"github.com/shamaton/msgpack/v3"
)

// TestErrorMessageFromBody locks the behaviour that error response bodies are
// decoded into a readable message rather than dumped as raw bytes — the bug
// being that msgpack-encoded {"error":"..."} bodies surfaced as garbled bytes
// in client error strings.
func TestErrorMessageFromBody(t *testing.T) {
	// JSON envelope
	got := errorMessageFromBody([]byte(`{"error":"remote manifest returned HTTP 404"}`))
	if got != "remote manifest returned HTTP 404" {
		t.Errorf("json envelope: got %q", got)
	}

	// Msgpack envelope (what the ApiClient actually receives)
	body, err := msgpack.Marshal(map[string]any{"error": "remote manifest returned HTTP 404"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = errorMessageFromBody(body)
	if got != "remote manifest returned HTTP 404" {
		t.Errorf("msgpack envelope: got %q", got)
	}

	// Plain text (no envelope) falls back to the raw body
	got = errorMessageFromBody([]byte("plain old text"))
	if got != "plain old text" {
		t.Errorf("plain fallback: got %q", got)
	}

	// Empty body
	got = errorMessageFromBody(nil)
	if got != "" {
		t.Errorf("empty body: got %q", got)
	}
}
