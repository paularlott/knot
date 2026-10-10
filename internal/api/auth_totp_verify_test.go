package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paularlott/knot/internal/authratelimit"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/totp"
)

// The web login asks a user to confirm a freshly generated TOTP secret by
// entering a code; the check must accept the current code, reject wrong
// ones (audit-logged, counted by the rate limiter) and refuse when blocked.
func TestVerifyTOTP(t *testing.T) {
	var entries []*model.AuditLogEntry
	model.AuditHook = func(e *model.AuditLogEntry) { entries = append(entries, e) }
	t.Cleanup(func() { model.AuditHook = nil })

	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		AuthIPRateLimiting:    true,
		AuthRateLimitAttempts: 2,
		TOTP:                  config.TOTPConfig{Enabled: true, Window: 1},
		Audit:                 config.AuditConfig{Routing: "external"},
		BadgerDB:              config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	const ip = "203.0.113.21"
	user := &model.User{Id: "user-totp", Username: "totp", Email: "totp@example.com", TOTPSecret: totp.GenerateSecret()}
	t.Cleanup(func() { authratelimit.Clear(ip, user.Email) })

	call := func(u *model.User, code string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(TOTPVerifyRequest{Code: code})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/totp/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":4321"
		req = req.WithContext(context.WithValue(req.Context(), "user", u))
		rec := httptest.NewRecorder()
		HandleVerifyTOTP(rec, req)
		return rec
	}

	good, err := totp.GetCode(user.TOTPSecret, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rec := call(user, good); rec.Code != http.StatusOK {
		t.Fatalf("current code should verify, got %d (%s)", rec.Code, rec.Body.String())
	}

	bad := "000000"
	if bad == good {
		bad = "111111"
	}
	if rec := call(user, bad); rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong code should be rejected with 400, got %d", rec.Code)
	}
	failed := false
	for _, e := range entries {
		if e.Event == model.AuditEventAuthFailed && e.Actor == user.Username {
			failed = true
		}
	}
	if !failed {
		t.Errorf("wrong code should be audit-logged as a failed login, got %+v", entries)
	}

	// A second failure reaches the configured limit; further attempts, even
	// with the right code, are refused until the block lifts.
	call(user, bad)
	if rec := call(user, good); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("blocked verification should return 429, got %d", rec.Code)
	}

	// Without a stored secret there is nothing to verify against.
	if rec := call(&model.User{Id: "u2", Username: "nosecret", Email: "n@example.com"}, good); rec.Code != http.StatusBadRequest {
		t.Fatalf("user without a secret should get 400, got %d", rec.Code)
	}
}
