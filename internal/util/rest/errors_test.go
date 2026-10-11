package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shamaton/msgpack/v3"
)

func TestHTTPErrorText(t *testing.T) {
	he := NewHTTPError(403, "POST", "/api/spaces/x/start", []byte(`{"error":"No permission to access this space"}`))
	// Error() keeps the historical prefix callers and scripts match on.
	if got, want := he.Error(), "unexpected status code: 403: No permission to access this space"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got, want := he.Message(), "No permission to access this space"; got != want {
		t.Errorf("Message() = %q, want %q", got, want)
	}
	if got, want := he.Summary(), "no permission to access this space (403)"; got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}

	empty := NewHTTPError(502, "GET", "/api/ping", nil)
	if got, want := empty.Error(), "unexpected status code: 502"; got != want {
		t.Errorf("empty Error() = %q, want %q", got, want)
	}
	if got, want := empty.Message(), "bad gateway"; got != want {
		t.Errorf("empty Message() = %q, want %q", got, want)
	}
	if got, want := empty.Summary(), "the server hit an internal error (502)"; got != want {
		t.Errorf("empty Summary() = %q, want %q", got, want)
	}

	// Acronyms at the start of a server message are left alone.
	acr := NewHTTPError(400, "PUT", "/", []byte(`{"error":"SSH key is invalid"}`))
	if got, want := acr.Summary(), "SSH key is invalid (400)"; got != want {
		t.Errorf("acronym Summary() = %q, want %q", got, want)
	}
}

func TestHTTPErrorHelpers(t *testing.T) {
	he := NewHTTPError(412, "PUT", "/x", nil)
	wrapped := fmt.Errorf("writing b:k: %w", he)
	if StatusOf(wrapped) != 412 {
		t.Errorf("StatusOf wrapped = %d", StatusOf(wrapped))
	}
	if !IsStatus(wrapped, 409, 412) || IsStatus(wrapped, 404) {
		t.Error("IsStatus mismatch")
	}
	if AsHTTPError(errors.New("plain")) != nil || StatusOf(nil) != 0 {
		t.Error("non-HTTP errors must not match")
	}
	// The legacy prefix check still works for code that hasn't moved to
	// errors.As yet (e.g. the Pro fork).
	if !strings.HasPrefix(wrapped.Error(), "writing b:k: unexpected status code: 412") {
		t.Errorf("legacy text = %q", wrapped.Error())
	}
}

// TestHTTPClientReturnsHTTPError checks every request path of HTTPClient
// produces an HTTPError carrying the status, the decoded server message, the
// method, path and server.
func TestHTTPClientReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ContentTypeMsgPack)
		w.WriteHeader(http.StatusNotFound)
		b, _ := msgpack.Marshal(map[string]string{"error": "space not found"})
		w.Write(b)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "tok", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	calls := map[string]func() error{
		"GET":  func() error { _, err := c.Get(ctx, "/api/spaces/x", nil); return err },
		"POST": func() error { _, err := c.Post(ctx, "/api/spaces/x", map[string]string{}, nil, 200); return err },
		"PUTJSON": func() error {
			_, err := c.PutJSON(ctx, "/api/spaces/x", map[string]string{}, nil, 200)
			return err
		},
		"GETJSON": func() error { _, err := c.GetJSON(ctx, "/api/spaces/x", nil); return err },
		"RAW": func() error {
			resp, err := c.DoRaw(ctx, http.MethodDelete, "/api/spaces/x", nil, 0, nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			return DecodeResponse(resp, nil)
		},
	}
	for name, call := range calls {
		err := call()
		he := AsHTTPError(err)
		if he == nil {
			t.Errorf("%s: want *HTTPError, got %T %v", name, err, err)
			continue
		}
		if he.StatusCode != 404 || he.ServerMessage != "space not found" || he.Path != "/api/spaces/x" || he.Server != srv.URL || he.Method == "" {
			t.Errorf("%s: unexpected fields %+v", name, he)
		}
	}
}
