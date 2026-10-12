package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
)

func TestReturnUnauthorizedMCPChallenge(t *testing.T) {
	prevCfg := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{URL: "https://knot.example.com"})
	t.Cleanup(func() { config.SetServerConfig(prevCfg) })

	rec := httptest.NewRecorder()
	returnUnauthorized(rec, httptest.NewRequest("POST", "/mcp", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	want := `resource_metadata="https://knot.example.com/.well-known/oauth-protected-resource/mcp"`
	if got := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer ") || !strings.Contains(got, want) {
		t.Errorf("WWW-Authenticate = %q, want Bearer challenge with %s", got, want)
	}

	rec = httptest.NewRecorder()
	returnUnauthorized(rec, httptest.NewRequest("GET", "/api/spaces", nil))
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Errorf("non-MCP WWW-Authenticate = %q, want none", got)
	}
}
