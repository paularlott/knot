package rest

import (
	"context"
	"net/http"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

// TestMuxClient_Get_RejectsMalformedPath verifies that a path containing
// raw whitespace returns a clean error instead of panicking.
//
// Regression: previously, MuxClient called httptest.NewRequest directly,
// which panics on paths like "/api/templates/ubuntu apple" because it
// builds an HTTP request-line and the space breaks version parsing.
// The panic surfaced as "panic in builtin: malformed HTTP version ..." to
// MCP tool callers.
func TestMuxClient_Get_RejectsMalformedPath(t *testing.T) {
	// Set up a minimal mux so MuxClient can be constructed.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	SetAPIMux(mux)

	client := NewMuxClient(&model.User{Id: "u1", Username: "tester"})

	// Whitespace in path must return error, not panic.
	for _, bad := range []string{
		"/api/templates/ubuntu apple",
		"/api/spaces/foo\tbar",
		"/api/spaces/foo\nbar",
	} {
		_, err := client.Get(context.Background(), bad, nil)
		if err == nil {
			t.Errorf("Get(%q) returned nil error; expected malformed-path error", bad)
		}
	}

	// Valid path still works.
	if _, err := client.Get(context.Background(), "/api/ok", nil); err != nil {
		t.Errorf("Get(/api/ok) returned error %v; expected nil", err)
	}
}

// TestNewMuxRequest_AcceptsEncodedPath verifies that an already-encoded path
// (the form produced by urllib.parse.quote) is accepted.
func TestNewMuxRequest_AcceptsEncodedPath(t *testing.T) {
	req, err := newMuxRequest(http.MethodGet, "/api/templates/ubuntu%20apple", nil)
	if err != nil {
		t.Fatalf("expected encoded path to be accepted, got error: %v", err)
	}
	if req.URL.Path != "/api/templates/ubuntu apple" {
		t.Errorf("decoded path = %q, want %q", req.URL.Path, "/api/templates/ubuntu apple")
	}
}

// TestNewMuxRequest_RejectsRawSpace verifies the unencoded form is rejected.
func TestNewMuxRequest_RejectsRawSpace(t *testing.T) {
	_, err := newMuxRequest(http.MethodGet, "/api/templates/ubuntu apple", nil)
	if err == nil {
		t.Fatal("expected error for path with raw space, got nil")
	}
}
