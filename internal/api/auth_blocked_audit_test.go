package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/authratelimit"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

// Attempts that arrive while the rate limiter has the IP or account blocked
// must land in the audit trail as Login Blocked — that is the evidence (and
// the signal) for continued attempts after lockout.
func TestBlockedLoginEmitsAudit(t *testing.T) {
	var entries []*model.AuditLogEntry
	model.AuditHook = func(e *model.AuditLogEntry) { entries = append(entries, e) }
	t.Cleanup(func() { model.AuditHook = nil })

	prev := config.GetServerConfig()
	// External routing keeps the audit write off the database; badger in a
	// temp dir satisfies the handler's driver initialisation.
	config.SetServerConfig(&config.ServerConfig{
		AuthIPRateLimiting: true,
		Audit:              config.AuditConfig{Routing: "external"},
		BadgerDB:           config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	authratelimit.ApplyEvent(&authratelimit.Event{IP: "203.0.113.9", At: time.Now(), BlockUntil: time.Now().Add(time.Minute)})
	t.Cleanup(func() { authratelimit.Clear("203.0.113.9", "") })

	body, err := json.Marshal(apiclient.AuthLoginRequest{Email: "alice@example.com", Password: "guess"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.9:1234"
	rec := httptest.NewRecorder()

	HandleAuthorization(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("blocked login should return 429, got %d (body %s)", rec.Code, rec.Body.String())
	}

	var blocked *model.AuditLogEntry
	for _, e := range entries {
		if e.Event == model.AuditEventAuthBlocked {
			blocked = e
		}
	}
	if blocked == nil {
		t.Fatalf("expected a Login Blocked audit entry, got %+v", entries)
	}
	if blocked.Actor != "alice@example.com" {
		t.Errorf("blocked entry actor should be the submitted email, got %q", blocked.Actor)
	}
	if blocked.Properties["source_ip"] != "203.0.113.9:1234" && blocked.Properties["source_ip"] != "203.0.113.9" {
		t.Errorf("blocked entry should carry the source ip, got %v", blocked.Properties["source_ip"])
	}
}
