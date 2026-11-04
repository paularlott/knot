package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A request whose context is marked WithoutTimeout may outlast the client's
// timeout; others may not, and the client itself is left as it was.
func TestWithoutTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c, err := NewClient(srv.URL, "token", true)
	if err != nil {
		t.Fatal(err)
	}
	c.SetTimeout(100 * time.Millisecond)

	var out struct{ OK bool }
	if _, err := c.Post(context.Background(), "/slow", map[string]string{}, &out, 200); err == nil {
		t.Fatal("a request past the client's timeout succeeded")
	}
	if _, err := c.Post(WithoutTimeout(context.Background()), "/slow", map[string]string{}, &out, 200); err != nil || !out.OK {
		t.Fatalf("a request marked WithoutTimeout failed: %v", err)
	}
	if c.HTTPClient.Timeout != 100*time.Millisecond {
		t.Errorf("the client's timeout changed to %v", c.HTTPClient.Timeout)
	}

	ctx, cancel := context.WithTimeout(WithoutTimeout(context.Background()), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Post(ctx, "/slow", map[string]string{}, &out, 200); err == nil {
		t.Error("a request marked WithoutTimeout outlasted its context")
	}
}
