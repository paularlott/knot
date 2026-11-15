package rest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/toolsurface"
)

// An in-process API call never carries the caller's tool-call surface, and
// honours the response limit on its context.
func TestMuxClientStripsSurfaceAndLimitsResponses(t *testing.T) {
	var seen []toolsurface.Surface
	mux := http.NewServeMux()
	mux.HandleFunc("/api/small", func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, toolsurface.From(r.Context()))
		if u, _ := r.Context().Value("user").(*model.User); u == nil || u.Id != "u1" {
			t.Errorf("user not in the request context")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	mux.HandleFunc("/api/big", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":"` + strings.Repeat("x", 2048) + `"}`))
	})
	old := apiMux
	SetAPIMux(mux)
	defer SetAPIMux(old)

	user := &model.User{Id: "u1"}
	client := NewMuxClient(user)
	ctx := toolsurface.With(context.Background(), toolsurface.WebChat)

	var out map[string]interface{}
	if _, err := client.Get(ctx, "/api/small", &out); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Post(ctx, "/api/small", map[string]string{}, &out, 0); err != nil {
		t.Fatal(err)
	}
	resp, err := client.(RawClient).DoRaw(ctx, http.MethodGet, "/api/small", nil, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://knot/api/small", nil)
	if _, err := NewMuxHTTPClient(user).Do(req); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 4 {
		t.Fatalf("handler saw %d calls, want 4", len(seen))
	}
	for i, s := range seen {
		if s != "" {
			t.Errorf("call %d saw surface %q", i, s)
		}
	}

	limited := WithResponseLimit(context.Background(), 1024)
	if _, err := client.Get(limited, "/api/big", &out); !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("Get over the limit: %v", err)
	}
	if _, err := client.(RawClient).DoRaw(limited, http.MethodGet, "/api/big", nil, 0, nil); !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("DoRaw over the limit: %v", err)
	}
	if _, err := client.Get(limited, "/api/small", &out); err != nil {
		t.Errorf("Get under the limit: %v", err)
	}
	if _, err := client.Get(context.Background(), "/api/big", &out); err != nil {
		t.Errorf("Get with no limit: %v", err)
	}
}
