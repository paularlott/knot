package apiclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusHelpers(t *testing.T) {
	cases := []struct {
		code  int
		check func(error) bool
	}{
		{401, IsUnauthorized},
		{403, IsForbidden},
		{404, IsNotFound},
		{409, IsConflict},
		{412, IsConflict},
		{429, IsTooManyRequests},
		{500, IsServerError},
		{503, IsServerError},
	}
	for _, c := range cases {
		err := fmt.Errorf("context: %w", newStatusError(c.code, "GET", "/x"))
		if !c.check(err) {
			t.Errorf("%d not recognised", c.code)
		}
		if StatusCode(err) != c.code {
			t.Errorf("StatusCode = %d, want %d", StatusCode(err), c.code)
		}
	}
	if IsServerError(newStatusError(404, "GET", "/")) || IsNotFound(errors.New("not found")) {
		t.Error("false positive")
	}
}

func TestIsPreconditionFailed(t *testing.T) {
	if !IsPreconditionFailed(fmt.Errorf("put: %w", newStatusError(412, "PUT", "/x"))) {
		t.Error("412 not recognised")
	}
	if IsPreconditionFailed(newStatusError(409, "PUT", "/x")) || IsPreconditionFailed(nil) {
		t.Error("false positive")
	}
}

// TestPingTypedError checks Ping now reports the server's status instead of
// the old "invalid status code".
func TestPingTypedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"authentication required"}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "bad", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Ping(context.Background())
	if !IsUnauthorized(err) {
		t.Fatalf("want 401 HTTPError, got %v", err)
	}
	if he := AsHTTPError(err); he.Message() != "authentication required" {
		t.Errorf("message = %q", he.Message())
	}
}
