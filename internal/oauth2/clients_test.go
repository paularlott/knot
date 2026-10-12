package oauth2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
)

func withEncryptionKey(t *testing.T, key string) {
	t.Helper()
	prevCfg := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{EncryptionKey: key, URL: "https://knot.example.com"})
	t.Cleanup(func() { config.SetServerConfig(prevCfg) })
}

func register(t *testing.T, body string) (*httptest.ResponseRecorder, clientRegistrationResponse) {
	t.Helper()
	req := httptest.NewRequest("POST", "/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	HandleRegister(rec, req)
	var resp clientRegistrationResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec, resp
}

func TestHandleRegister(t *testing.T) {
	withEncryptionKey(t, "test-key")

	tests := []struct {
		name string
		body string
		want int
	}{
		{"claude connector", `{"client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback"],"grant_types":["authorization_code","refresh_token"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, http.StatusCreated},
		{"loopback http", `{"redirect_uris":["http://127.0.0.1:33418/callback"]}`, http.StatusCreated},
		{"localhost http", `{"redirect_uris":["http://localhost:8080/callback"]}`, http.StatusCreated},
		{"no redirect uris", `{"client_name":"x"}`, http.StatusBadRequest},
		{"remote http rejected", `{"redirect_uris":["http://evil.example.com/cb"]}`, http.StatusBadRequest},
		{"custom scheme rejected", `{"redirect_uris":["javascript:alert(1)"]}`, http.StatusBadRequest},
		{"confidential client rejected", `{"redirect_uris":["https://a.example.com/cb"],"token_endpoint_auth_method":"client_secret_basic"}`, http.StatusBadRequest},
		{"client credentials rejected", `{"redirect_uris":["https://a.example.com/cb"],"grant_types":["client_credentials"]}`, http.StatusBadRequest},
		{"malformed json", `{`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, resp := register(t, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
			if tt.want == http.StatusCreated && !strings.HasPrefix(resp.ClientId, registeredClientPrefix) {
				t.Errorf("client_id = %q, want %s prefix", resp.ClientId, registeredClientPrefix)
			}
		})
	}
}

func TestValidateClient(t *testing.T) {
	withEncryptionKey(t, "test-key")

	_, resp := register(t, `{"client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback"]}`)

	name, err := ValidateClient(resp.ClientId, "https://claude.ai/api/mcp/auth_callback")
	if err != nil || name != "Claude" {
		t.Errorf("registered redirect: name=%q err=%v", name, err)
	}

	if _, err := ValidateClient(resp.ClientId, "https://evil.example.com/cb"); err == nil {
		t.Error("unregistered redirect_uri accepted")
	}

	// A client_id tampered to add a redirect URI fails the signature.
	forged, _ := encodeClientId(registeredClient{Name: "Claude", RedirectURIs: []string{"https://evil.example.com/cb"}})
	payload, _, _ := strings.Cut(strings.TrimPrefix(forged, registeredClientPrefix), ".")
	_, origSig, _ := strings.Cut(resp.ClientId, ".")
	if _, err := ValidateClient(registeredClientPrefix+payload+"."+origSig, "https://evil.example.com/cb"); err == nil {
		t.Error("tampered client_id accepted")
	}

	// A client registered under another zone's key is unknown here.
	withEncryptionKey(t, "other-key")
	if _, err := ValidateClient(resp.ClientId, "https://claude.ai/api/mcp/auth_callback"); err == nil {
		t.Error("client_id signed with another key accepted")
	}

	// Unregistered client IDs keep working.
	if name, err := ValidateClient("test-client", "http://localhost:8080/callback"); err != nil || name != "" {
		t.Errorf("unregistered client: name=%q err=%v", name, err)
	}
}

func TestHandleAuthorizeRegisteredClient(t *testing.T) {
	withEncryptionKey(t, "test-key")

	_, resp := register(t, `{"client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback"]}`)
	pkce := "&code_challenge=" + s256Challenge("verifier-verifier-verifier-123456") + "&code_challenge_method=S256"
	base := "/authorize?response_type=code&client_id=" + url.QueryEscape(resp.ClientId) + pkce

	tests := []struct {
		name     string
		redirect string
		want     int
	}{
		{"registered redirect", "https://claude.ai/api/mcp/auth_callback", http.StatusSeeOther},
		{"other redirect", "https://evil.example.com/cb", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", base+"&redirect_uri="+url.QueryEscape(tt.redirect), nil)
			rec := httptest.NewRecorder()
			HandleAuthorize(rec, req)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestDiscoveryMetadata(t *testing.T) {
	withEncryptionKey(t, "test-key")

	rec := httptest.NewRecorder()
	HandleProtectedResourceMetadata(rec, httptest.NewRequest("GET", "/.well-known/oauth-protected-resource/mcp", nil))
	var prm ProtectedResourceMetadata
	json.Unmarshal(rec.Body.Bytes(), &prm)
	if prm.Resource != "https://knot.example.com/mcp" || len(prm.AuthorizationServers) != 1 || prm.AuthorizationServers[0] != "https://knot.example.com" {
		t.Errorf("protected resource metadata = %+v", prm)
	}

	rec = httptest.NewRecorder()
	HandleAuthorizationServerMetadata(rec, httptest.NewRequest("GET", "/.well-known/oauth-authorization-server", nil))
	var asm AuthorizationServerMetadata
	json.Unmarshal(rec.Body.Bytes(), &asm)
	if asm.RegistrationEndpoint != "https://knot.example.com/register" {
		t.Errorf("registration_endpoint = %q", asm.RegistrationEndpoint)
	}
}
