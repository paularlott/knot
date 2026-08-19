package oauth2

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
)

func s256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestValidatePKCEChallenge(t *testing.T) {
	valid := s256Challenge("a-verifier-with-enough-entropy-1234567890")

	tests := []struct {
		name      string
		challenge string
		method    string
		wantErr   bool
	}{
		{"valid s256", valid, "S256", false},
		{"missing challenge", "", "S256", true},
		{"missing method defaults to plain rejected", valid, "", true},
		{"plain method rejected", valid, "plain", true},
		{"unknown method rejected", valid, "XX256", true},
		{"challenge too short", "short", "S256", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := validatePKCEChallenge(tt.challenge, tt.method)
			if (reason != "") != tt.wantErr {
				t.Errorf("validatePKCEChallenge(%q, %q) = %q, wantErr %v", tt.challenge, tt.method, reason, tt.wantErr)
			}
		})
	}
}

func TestVerifyPKCE(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := s256Challenge(verifier)

	if !verifyPKCE(challenge, "S256", verifier) {
		t.Error("correct verifier should pass")
	}
	if verifyPKCE(challenge, "S256", verifier+"x") {
		t.Error("wrong verifier should fail")
	}
	if verifyPKCE(challenge, "S256", "") {
		t.Error("empty verifier should fail")
	}
	if verifyPKCE("", "S256", verifier) {
		t.Error("missing stored challenge should fail")
	}
	if verifyPKCE(verifier, "plain", verifier) {
		t.Error("plain method must never verify")
	}
}

func TestGrantedScopes(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		want  []string
	}{
		{"empty defaults to mcp", "", []string{model.ScopeMCP}},
		{"unknown defaults to mcp", "read write profile", []string{model.ScopeMCP}},
		{"mcp granted", "mcp", []string{model.ScopeMCP}},
		{"mixed keeps known only", "openid mcp methods", []string{model.ScopeMCP, model.ScopeMethods}},
		{"methods granted", "methods", []string{model.ScopeMethods}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := grantedScopes(tt.scope)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("grantedScopes(%q) = %v, want %v", tt.scope, got, tt.want)
			}
		})
	}
}

func TestHandleAuthorizeRequiresPKCE(t *testing.T) {
	base := "/authorize?response_type=code&client_id=test-client&redirect_uri=http://localhost:8080/callback"

	tests := []struct {
		name string
		url  string
		want int
	}{
		{"missing code_challenge", base, http.StatusBadRequest},
		{"plain method rejected", base + "&code_challenge=" + s256Challenge("verifier-verifier-verifier-123456") + "&code_challenge_method=plain", http.StatusBadRequest},
		{"valid challenge", base + "&code_challenge=" + s256Challenge("verifier-verifier-verifier-123456") + "&code_challenge_method=S256", http.StatusSeeOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			rec := httptest.NewRecorder()
			HandleAuthorize(rec, req)
			if rec.Code != tt.want {
				t.Errorf("HandleAuthorize(%q) = %d, want %d", tt.url, rec.Code, tt.want)
			}
		})
	}
}

func TestAuthCodeRoundTripPKCEFields(t *testing.T) {
	store := GetAuthCodeStore()
	challenge := s256Challenge("round-trip-verifier-1234567890")

	authCode, err := store.CreateAuthCode("user-1", "client-1", "http://localhost/cb", "mcp", challenge, "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode failed: %v", err)
	}

	stored, ok := store.ConsumeAuthCode(authCode.Code)
	if !ok {
		t.Fatal("ConsumeAuthCode failed")
	}
	if stored.CodeChallenge != challenge || stored.CodeChallengeMethod != "S256" {
		t.Errorf("PKCE fields not preserved: %+v", stored)
	}
	if !verifyPKCE(stored.CodeChallenge, stored.CodeChallengeMethod, "round-trip-verifier-1234567890") {
		t.Error("stored challenge should verify against the original verifier")
	}
}

type gossipCaptureTransport struct {
	service.Transport
	tokens int
}

func (t *gossipCaptureTransport) GossipToken(*model.Token) { t.tokens++ }

func TestRefreshTokenGrantGuards(t *testing.T) {
	prevCfg := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prevCfg) })

	transport := &gossipCaptureTransport{}
	service.SetTransport(transport)
	t.Cleanup(func() { service.SetTransport(nil) })

	db := database.GetInstance()

	newRefreshReq := func(id string) *http.Request {
		req := httptest.NewRequest("POST", "/token", strings.NewReader("grant_type=refresh_token&refresh_token="+id))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req
	}

	t.Run("non oauth token is rejected", func(t *testing.T) {
		token := model.NewToken("UI token", "user-1")
		if err := db.SaveToken(token); err != nil {
			t.Fatal(err)
		}

		rec := httptest.NewRecorder()
		handleRefreshTokenGrant(rec, newRefreshReq(token.Id))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("non-OAuth token refresh = %d, want 400", rec.Code)
		}
	})

	t.Run("expired oauth token is rejected", func(t *testing.T) {
		token := model.NewToken("stale", "user-1")
		token.RefreshToken = true
		token.ExpiresAfter = time.Now().UTC().Add(-time.Hour)
		if err := db.SaveToken(token); err != nil {
			t.Fatal(err)
		}

		rec := httptest.NewRecorder()
		handleRefreshTokenGrant(rec, newRefreshReq(token.Id))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expired token refresh = %d, want 400 (no resurrection)", rec.Code)
		}
	})

	t.Run("valid oauth token is extended", func(t *testing.T) {
		token := model.NewToken("live", "user-1")
		token.RefreshToken = true
		before := token.ExpiresAfter
		if err := db.SaveToken(token); err != nil {
			t.Fatal(err)
		}

		rec := httptest.NewRecorder()
		handleRefreshTokenGrant(rec, newRefreshReq(token.Id))
		if rec.Code != http.StatusOK {
			t.Fatalf("valid refresh = %d, want 200 (body %s)", rec.Code, rec.Body.String())
		}

		updated, err := db.GetToken(token.Id)
		if err != nil {
			t.Fatal(err)
		}
		if !updated.ExpiresAfter.After(before) {
			t.Error("expiry should have been extended")
		}
		if transport.tokens == 0 {
			t.Error("token gossip should have fired")
		}
	})

	t.Run("unknown token is rejected", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handleRefreshTokenGrant(rec, newRefreshReq("no-such-token"))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("unknown token refresh = %d, want 400", rec.Code)
		}
	})
}
