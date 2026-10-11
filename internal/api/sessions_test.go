package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

// A request authenticated by an API token carries a user but no session, so
// listing sessions must not assume one.
func TestGetSessionsWithoutSession(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
		Zone:     "z-sessions",
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	user := &model.User{Id: "sess-user", Username: "sessuser", Active: true}
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Accept", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "user", user))
	rec := httptest.NewRecorder()

	HandleGetSessions(rec, req)

	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("got %d %q, want 200 []", rec.Code, rec.Body.String())
	}
}
