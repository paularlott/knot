package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

func TestReturnUnauthorizedMCPChallenge(t *testing.T) {
	prevCfg := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{URL: "https://knot.example.com"})
	t.Cleanup(func() { config.SetServerConfig(prevCfg) })

	const metadata = `resource_metadata="https://knot.example.com/.well-known/oauth-protected-resource/mcp"`
	const scope = `scope="` + model.ScopeMCP + `"`

	// No credentials: a challenge naming the metadata and scope, no error code.
	rec := httptest.NewRecorder()
	returnUnauthorized(rec, httptest.NewRequest("POST", "/mcp", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	got := rec.Header().Get("WWW-Authenticate")
	if !strings.HasPrefix(got, "Bearer ") || !strings.Contains(got, metadata) || !strings.Contains(got, scope) {
		t.Errorf("WWW-Authenticate = %q, want Bearer challenge with %s and %s", got, metadata, scope)
	}
	if strings.Contains(got, "error=") {
		t.Errorf("WWW-Authenticate = %q, want no error code without a token", got)
	}

	// A refused bearer token: error="invalid_token".
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer not-a-token")
	returnUnauthorized(rec, req)
	got = rec.Header().Get("WWW-Authenticate")
	if rec.Code != http.StatusUnauthorized || !strings.Contains(got, metadata) || !strings.Contains(got, `error="invalid_token"`) {
		t.Errorf("status = %d, WWW-Authenticate = %q, want 401 with invalid_token", rec.Code, got)
	}

	rec = httptest.NewRecorder()
	returnUnauthorized(rec, httptest.NewRequest("GET", "/api/spaces", nil))
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Errorf("non-MCP WWW-Authenticate = %q, want none", got)
	}
}

func TestReturnScopeDeniedMCP(t *testing.T) {
	prevCfg := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{URL: "https://knot.example.com"})
	t.Cleanup(func() { config.SetServerConfig(prevCfg) })

	rec := httptest.NewRecorder()
	returnScopeDenied(rec, httptest.NewRequest("POST", "/mcp", nil))
	got := rec.Header().Get("WWW-Authenticate")
	if rec.Code != http.StatusForbidden || !strings.Contains(got, `error="insufficient_scope"`) || !strings.Contains(got, `scope="`+model.ScopeMCP+`"`) {
		t.Errorf("status = %d, WWW-Authenticate = %q, want 403 insufficient_scope naming %s", rec.Code, got, model.ScopeMCP)
	}

	rec = httptest.NewRecorder()
	returnScopeDenied(rec, httptest.NewRequest("GET", "/api/spaces", nil))
	if rec.Code != http.StatusForbidden || rec.Header().Get("WWW-Authenticate") != "" {
		t.Errorf("non-MCP: status = %d, WWW-Authenticate = %q, want 403 and none", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
}

// A non-Bearer Authorization header is refused with a single response, not
// two (GetBearerToken used to write one and ApiAuth another).
func TestApiAuthNonBearerWritesOnce(t *testing.T) {
	prevHas := HasUsers
	HasUsers = true
	t.Cleanup(func() { HasUsers = prevHas })
	prevCfg := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{URL: "https://knot.example.com"})
	t.Cleanup(func() { config.SetServerConfig(prevCfg) })

	rec := &countingRecorder{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest("POST", "/mcp", nil).WithContext(context.Background())
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	ApiAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler reached without credentials")
	})(rec, req)
	if rec.Code != http.StatusUnauthorized || rec.writes != 1 {
		t.Errorf("status = %d, WriteHeader calls = %d, want 401 once", rec.Code, rec.writes)
	}
	if got := rec.Header().Get("WWW-Authenticate"); strings.Contains(got, "error=") {
		t.Errorf("WWW-Authenticate = %q, want no error code (no bearer token was sent)", got)
	}
}

type countingRecorder struct {
	*httptest.ResponseRecorder
	writes int
}

func (c *countingRecorder) WriteHeader(code int) {
	c.writes++
	c.ResponseRecorder.WriteHeader(code)
}
