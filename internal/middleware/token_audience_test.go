package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// A token whose recorded audience is another resource is refused at /mcp
// with 401 invalid_token; a token for this MCP endpoint, or with no recorded
// audience (issued before audiences were), is let through.
func TestApiAuthTokenAudience(t *testing.T) {
	prevHas := HasUsers
	HasUsers = true
	t.Cleanup(func() { HasUsers = prevHas })
	prevCfg := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		URL:      "https://knot.example.com",
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prevCfg) })

	db := database.GetInstance()
	if err := db.SaveUser(&model.User{Id: "aud-user", Username: "aud", Email: "aud@example.com", Active: true}, nil); err != nil {
		t.Fatal(err)
	}
	newToken := func(audience string) string {
		token := model.NewToken("oauth", "aud-user")
		token.Scopes = []string{model.ScopeMCP}
		token.RefreshToken = true
		token.Audience = audience
		if err := db.SaveToken(token); err != nil {
			t.Fatal(err)
		}
		return token.Id
	}

	tests := []struct {
		name     string
		audience string
		allowed  bool
	}{
		{"this MCP endpoint", "https://knot.example.com/mcp", true},
		{"this MCP endpoint, trailing slash", "https://knot.example.com/mcp/", true},
		{"no recorded audience", "", true},
		{"another server", "https://other.example.com/mcp", false},
		{"another resource on this server", "https://knot.example.com/api", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/mcp", nil)
			req.Header.Set("Authorization", "Bearer "+newToken(tt.audience))
			rec := httptest.NewRecorder()
			reached := false
			ApiAuth(func(w http.ResponseWriter, r *http.Request) { reached = true })(rec, req)

			if reached != tt.allowed {
				t.Fatalf("handler reached = %v, want %v (status %d)", reached, tt.allowed, rec.Code)
			}
			if !tt.allowed {
				got := rec.Header().Get("WWW-Authenticate")
				if rec.Code != http.StatusUnauthorized || !strings.Contains(got, `error="invalid_token"`) || !strings.Contains(got, "resource_metadata=") {
					t.Errorf("status = %d, WWW-Authenticate = %q, want 401 invalid_token", rec.Code, got)
				}
			}
		})
	}
}
