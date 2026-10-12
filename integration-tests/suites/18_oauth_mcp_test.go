//go:build integration

package suites

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/integration-tests/harness"
)

// oauthBrowser is a cookie-holding client that, like an OAuth client's
// browser, doesn't follow redirects so each hop can be checked.
func oauthBrowser(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func oauthDo(t *testing.T, c *http.Client, req *http.Request) (*http.Response, string) {
	t.Helper()
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func oauthGetJSON(t *testing.T, c *http.Client, u string, out any) {
	t.Helper()
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Accept", "application/json")
	resp, body := oauthDo(t, c, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", u, resp.StatusCode, body)
	}
	if err := json.Unmarshal([]byte(body), out); err != nil {
		t.Fatalf("GET %s: decode %q: %v", u, body, err)
	}
}

// oauthWebLogin signs the browser in to the web UI with a session cookie.
func oauthWebLogin(t *testing.T, c *http.Client, email, password string) {
	t.Helper()
	data, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, _ := http.NewRequest("POST", server.BaseURL+"/api/auth/web", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, body := oauthDo(t, c, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("web login = %d: %s", resp.StatusCode, body)
	}
}

type oauthTokens struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
}

func oauthTokenRequest(t *testing.T, form url.Values) (int, oauthTokens) {
	t.Helper()
	req, _ := http.NewRequest("POST", server.BaseURL+"/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, body := oauthDo(t, http.DefaultClient, req)
	var tokens oauthTokens
	json.Unmarshal([]byte(body), &tokens)
	return resp.StatusCode, tokens
}

// mcpCall posts one JSON-RPC request to /mcp and returns the status, headers
// and body.
func mcpCall(t *testing.T, token, method string, params any) (int, http.Header, string) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, _ := http.NewRequest("POST", server.BaseURL+"/mcp", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, body := oauthDo(t, http.DefaultClient, req)
	return resp.StatusCode, resp.Header, body
}

var mcpInitParams = map[string]any{
	"protocolVersion": "2025-06-18",
	"capabilities":    map[string]any{},
	"clientInfo":      map[string]any{"name": "knot-integration", "version": "1"},
}

// oauthAuthorize runs authorize → grant page → approve as the signed-in
// browser and returns the authorization code sent to the redirect URI.
func oauthAuthorize(t *testing.T, c *http.Client, clientId, redirectURI, challenge, wantName string) string {
	t.Helper()
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientId},
		"redirect_uri":          {redirectURI},
		"scope":                 {"mcp"},
		"state":                 {"st-123"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}

	req, _ := http.NewRequest("GET", server.BaseURL+"/authorize?"+q.Encode(), nil)
	resp, body := oauthDo(t, c, req)
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/oauth/grant?") {
		t.Fatalf("authorize = %d to %q, want 303 to the grant page: %s", resp.StatusCode, resp.Header.Get("Location"), body)
	}

	req, _ = http.NewRequest("GET", server.BaseURL+resp.Header.Get("Location"), nil)
	resp, body = oauthDo(t, c, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("grant page = %d: %s", resp.StatusCode, body)
	}
	if wantName != "" && !strings.Contains(body, "<strong>"+wantName+"</strong>") {
		t.Errorf("grant page does not name the client %q", wantName)
	}

	form := url.Values{
		"action":                {"approve"},
		"client_id":             {clientId},
		"redirect_uri":          {redirectURI},
		"scope":                 {"mcp"},
		"state":                 {"st-123"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	req, _ = http.NewRequest("POST", server.BaseURL+"/oauth/grant", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, body = oauthDo(t, c, req)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("approve = %d: %s", resp.StatusCode, body)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("approve redirect %q: %v", resp.Header.Get("Location"), err)
	}
	if !strings.HasPrefix(loc.String(), redirectURI+"?") {
		t.Fatalf("approve redirected to %q, want %s", loc, redirectURI)
	}
	mustEqual(t, "state returned", loc.Query().Get("state"), "st-123")
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatalf("approve redirect %q has no code", loc)
	}
	return code
}

// oauthExchangeAndUse swaps the code for a token, uses it on /mcp, and
// refreshes it.
func oauthExchangeAndUse(t *testing.T, code, redirectURI, verifier string) {
	t.Helper()
	status, tokens := oauthTokenRequest(t, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
	if status != http.StatusOK || tokens.AccessToken == "" {
		t.Fatalf("token exchange = %d %+v", status, tokens)
	}
	mustEqual(t, "token type", tokens.TokenType, "Bearer")
	mustEqual(t, "token scope", tokens.Scope, "mcp")

	status, _, body := mcpCall(t, tokens.AccessToken, "initialize", mcpInitParams)
	if status != http.StatusOK || !strings.Contains(body, `"serverInfo"`) {
		t.Fatalf("initialize with OAuth token = %d: %s", status, body)
	}
	status, _, body = mcpCall(t, tokens.AccessToken, "tools/list", map[string]any{})
	if status != http.StatusOK || !strings.Contains(body, `"tools"`) {
		t.Fatalf("tools/list with OAuth token = %d: %s", status, body)
	}

	// The token is scoped to MCP: the rest of the API is closed to it.
	req, _ := http.NewRequest("GET", server.BaseURL+"/api/spaces", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	resp, _ := oauthDo(t, http.DefaultClient, req)
	mustEqual(t, "OAuth token on /api/spaces", resp.StatusCode, http.StatusForbidden)

	// The code is single use.
	status, _ = oauthTokenRequest(t, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
	mustEqual(t, "code reuse", status, http.StatusBadRequest)

	status, refreshed := oauthTokenRequest(t, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tokens.RefreshToken},
	})
	if status != http.StatusOK || refreshed.AccessToken == "" {
		t.Fatalf("refresh = %d %+v", status, refreshed)
	}
}

func pkcePair(seed string) (verifier, challenge string) {
	verifier = "it-verifier-" + seed + "-0123456789abcdefghijklmnopqrstuvwxyz"
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestOAuthMCPDiscovery(t *testing.T) {
	harness.Feature(t, "mcp")

	// An unauthenticated MCP request is told where to authorize.
	status, header, body := mcpCall(t, "", "initialize", mcpInitParams)
	mustEqual(t, "unauthenticated /mcp", status, http.StatusUnauthorized)
	challenge := header.Get("WWW-Authenticate")
	m := regexp.MustCompile(`^Bearer .*resource_metadata="([^"]+)"`).FindStringSubmatch(challenge)
	if m == nil {
		t.Fatalf("WWW-Authenticate = %q (body %s), want a Bearer challenge with resource_metadata", challenge, body)
	}

	// The metadata URL is built from the server's public URL; fetch the same
	// path from the test server.
	metaURL, err := url.Parse(m[1])
	if err != nil || metaURL.Path != "/.well-known/oauth-protected-resource/mcp" {
		t.Fatalf("resource_metadata = %q", m[1])
	}
	var prm struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		ScopesSupported      []string `json:"scopes_supported"`
	}
	oauthGetJSON(t, http.DefaultClient, server.BaseURL+metaURL.Path, &prm)
	if !strings.HasSuffix(prm.Resource, "/mcp") || len(prm.AuthorizationServers) != 1 {
		t.Fatalf("protected resource metadata = %+v", prm)
	}
	issuer := prm.AuthorizationServers[0]

	var asm struct {
		Issuer                string `json:"issuer"`
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		RegistrationEndpoint  string `json:"registration_endpoint"`
	}
	oauthGetJSON(t, http.DefaultClient, server.BaseURL+"/.well-known/oauth-authorization-server", &asm)
	mustEqual(t, "issuer", asm.Issuer, issuer)
	mustEqual(t, "authorization_endpoint", asm.AuthorizationEndpoint, issuer+"/authorize")
	mustEqual(t, "token_endpoint", asm.TokenEndpoint, issuer+"/token")
	mustEqual(t, "registration_endpoint", asm.RegistrationEndpoint, issuer+"/register")

	// GET and DELETE are routed and authenticated too.
	for _, method := range []string{"GET", "DELETE"} {
		req, _ := http.NewRequest(method, server.BaseURL+"/mcp", nil)
		resp, _ := oauthDo(t, http.DefaultClient, req)
		mustEqual(t, "unauthenticated "+method+" /mcp", resp.StatusCode, http.StatusUnauthorized)
	}
	req, _ := http.NewRequest("GET", server.BaseURL+"/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+admin.Token)
	resp, _ := oauthDo(t, http.DefaultClient, req)
	mustEqual(t, "plain GET /mcp", resp.StatusCode, http.StatusMethodNotAllowed)

	// A token still works the way it always has.
	status, _, body = mcpCall(t, admin.Token, "initialize", mcpInitParams)
	if status != http.StatusOK {
		t.Fatalf("initialize with API token = %d: %s", status, body)
	}
}

// TestOAuthUnregisteredClient is the flow clients used before dynamic
// registration: any client_id, PKCE, and the code sent to its redirect URI.
func TestOAuthUnregisteredClient(t *testing.T) {
	harness.Feature(t, "auth")
	browser := oauthBrowser(t)
	oauthWebLogin(t, browser, admin.Email, admin.Password)

	redirectURI := "http://127.0.0.1:9/callback"
	verifier, challenge := pkcePair("legacy")
	code := oauthAuthorize(t, browser, "it-legacy-client", redirectURI, challenge, "")
	oauthExchangeAndUse(t, code, redirectURI, verifier)

	// A wrong verifier is refused.
	code = oauthAuthorize(t, browser, "it-legacy-client", redirectURI, challenge, "")
	status, _ := oauthTokenRequest(t, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {"not-the-verifier-0123456789abcdefghijklmnopqrstuvwxyz"},
	})
	mustEqual(t, "wrong PKCE verifier", status, http.StatusBadRequest)
}

// TestOAuthRegisteredClient is the flow of a client that registers itself,
// as Claude's custom connectors do.
func TestOAuthRegisteredClient(t *testing.T) {
	harness.Feature(t, "auth")

	redirectURI := "https://client.example.test/api/mcp/auth_callback"
	data, _ := json.Marshal(map[string]any{
		"client_name":                "Integration Client",
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
	req, _ := http.NewRequest("POST", server.BaseURL+"/register", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, body := oauthDo(t, http.DefaultClient, req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register = %d: %s", resp.StatusCode, body)
	}
	var reg struct {
		ClientId string `json:"client_id"`
	}
	json.Unmarshal([]byte(body), &reg)
	if reg.ClientId == "" {
		t.Fatalf("register returned no client_id: %s", body)
	}

	// Registration refuses a redirect URI that isn't https or loopback.
	req, _ = http.NewRequest("POST", server.BaseURL+"/register", strings.NewReader(`{"redirect_uris":["http://evil.example.test/cb"]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = oauthDo(t, http.DefaultClient, req)
	mustEqual(t, "register plain http redirect", resp.StatusCode, http.StatusBadRequest)

	browser := oauthBrowser(t)
	oauthWebLogin(t, browser, admin.Email, admin.Password)

	verifier, challenge := pkcePair("registered")
	code := oauthAuthorize(t, browser, reg.ClientId, redirectURI, challenge, "Integration Client")
	oauthExchangeAndUse(t, code, redirectURI, verifier)

	// The registered client's codes only go to its own redirect URI, whether
	// asked through authorize or a forged approval.
	evil := "https://evil.example.test/cb"
	q := url.Values{
		"response_type": {"code"}, "client_id": {reg.ClientId}, "redirect_uri": {evil},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}
	req, _ = http.NewRequest("GET", server.BaseURL+"/authorize?"+q.Encode(), nil)
	resp, _ = oauthDo(t, browser, req)
	mustEqual(t, "authorize to unregistered redirect", resp.StatusCode, http.StatusBadRequest)

	form := url.Values{
		"action": {"approve"}, "client_id": {reg.ClientId}, "redirect_uri": {evil},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}
	req, _ = http.NewRequest("POST", server.BaseURL+"/oauth/grant", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ = oauthDo(t, browser, req)
	mustEqual(t, "forged approval to unregistered redirect", resp.StatusCode, http.StatusBadRequest)

	// A tampered client_id is unknown.
	q.Set("client_id", reg.ClientId+"x")
	q.Set("redirect_uri", redirectURI)
	req, _ = http.NewRequest("GET", server.BaseURL+"/authorize?"+q.Encode(), nil)
	resp, _ = oauthDo(t, browser, req)
	mustEqual(t, "authorize with tampered client_id", resp.StatusCode, http.StatusBadRequest)
}
