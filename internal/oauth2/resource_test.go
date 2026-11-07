package oauth2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/paularlott/gossip/codec/shamaton"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
)

const testResource = "https://knot.example.com/mcp"

func TestMatchesMCPResource(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		uri       string
		want      bool
	}{
		{"exact", "https://knot.example.com", testResource, true},
		{"trailing slash", "https://knot.example.com", testResource + "/", true},
		{"case-insensitive scheme and host", "https://knot.example.com", "HTTPS://Knot.Example.COM/mcp", true},
		{"other host", "https://knot.example.com", "https://evil.example.com/mcp", false},
		{"other path", "https://knot.example.com", "https://knot.example.com/api", false},
		{"path is case-sensitive", "https://knot.example.com", "https://knot.example.com/MCP", false},
		{"query", "https://knot.example.com", testResource + "?x=1", false},
		{"fragment", "https://knot.example.com", testResource + "#x", false},
		{"userinfo", "https://knot.example.com", "https://u@knot.example.com/mcp", false},
		{"other port", "https://knot.example.com", "https://knot.example.com:8443/mcp", false},
		{"configured url: scheme compared", "https://knot.example.com", "http://knot.example.com/mcp", false},
		// Without server.url the scheme comes from the request, which is
		// wrong behind a TLS-terminating proxy, so it is not compared.
		{"no configured url: https accepted", "", "https://knot.example.com/mcp", true},
		{"no configured url: http accepted", "", "http://knot.example.com/mcp", true},
		{"no configured url: other host", "", "https://evil.example.com/mcp", false},
		{"no configured url: other scheme", "", "ftp://knot.example.com/mcp", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := config.GetServerConfig()
			config.SetServerConfig(&config.ServerConfig{URL: tt.serverURL})
			t.Cleanup(func() { config.SetServerConfig(prev) })

			// The request reaches the server over plain http, as it does
			// behind a TLS-terminating proxy.
			req := httptest.NewRequest("POST", "http://knot.example.com/mcp", nil)
			if got := MatchesMCPResource(req, tt.uri); got != tt.want {
				t.Errorf("MatchesMCPResource(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

func TestValidResourceIndicator(t *testing.T) {
	for s, want := range map[string]bool{
		testResource:       true,
		"":                 false,
		"/mcp":             false,
		"mcp":              false,
		testResource + "#": false,
		"urn:example:mcp":  false, // absolute, but not a resource knot can compare
	} {
		if got := validResourceIndicator(s); got != want {
			t.Errorf("validResourceIndicator(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestHandleAuthorizeResource(t *testing.T) {
	withEncryptionKey(t, "test-key")

	pkce := "&code_challenge=" + s256Challenge("verifier-verifier-verifier-123456") + "&code_challenge_method=S256"
	unregistered := "/authorize?response_type=code&client_id=test-client&redirect_uri=" + url.QueryEscape("http://localhost:8080/callback") + pkce + "&state=st"

	_, reg := register(t, `{"client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback"]}`)
	registered := "/authorize?response_type=code&client_id=" + url.QueryEscape(reg.ClientId) +
		"&redirect_uri=" + url.QueryEscape("https://claude.ai/api/mcp/auth_callback") + pkce + "&state=st"

	authorize := func(u string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		HandleAuthorize(rec, httptest.NewRequest("GET", u, nil))
		return rec
	}

	t.Run("no resource", func(t *testing.T) {
		if rec := authorize(unregistered); rec.Code != http.StatusSeeOther {
			t.Errorf("status = %d, want 303", rec.Code)
		}
	})
	t.Run("matching resource", func(t *testing.T) {
		rec := authorize(unregistered + "&resource=" + url.QueryEscape(testResource+"/"))
		if rec.Code != http.StatusSeeOther {
			t.Errorf("status = %d (%s), want 303", rec.Code, rec.Body.String())
		}
	})
	t.Run("foreign resource, unregistered client", func(t *testing.T) {
		rec := authorize(unregistered + "&resource=" + url.QueryEscape("https://other.example.com/mcp"))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), ErrorInvalidTarget) {
			t.Errorf("status = %d (%s), want 400 invalid_target", rec.Code, rec.Body.String())
		}
	})
	t.Run("relative resource", func(t *testing.T) {
		rec := authorize(unregistered + "&resource=%2Fmcp")
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), ErrorInvalidTarget) {
			t.Errorf("status = %d (%s), want 400 invalid_target", rec.Code, rec.Body.String())
		}
	})
	t.Run("one of several resources foreign", func(t *testing.T) {
		rec := authorize(unregistered + "&resource=" + url.QueryEscape(testResource) + "&resource=" + url.QueryEscape("https://other.example.com/"))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
	t.Run("foreign resource, registered client is redirected the error", func(t *testing.T) {
		rec := authorize(registered + "&resource=" + url.QueryEscape("https://other.example.com/mcp"))
		loc, _ := url.Parse(rec.Header().Get("Location"))
		if rec.Code != http.StatusSeeOther || loc == nil || loc.Host != "claude.ai" ||
			loc.Query().Get("error") != ErrorInvalidTarget || loc.Query().Get("state") != "st" {
			t.Errorf("status = %d, Location = %q, want 303 to the redirect URI with error=invalid_target", rec.Code, rec.Header().Get("Location"))
		}
	})
}

// grantCode posts an approval to HandleGrant and returns the code it issues.
func grantCode(t *testing.T, form url.Values) (*httptest.ResponseRecorder, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/oauth/grant", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(contextWithUser(req, &model.User{Id: "user-1"}))
	rec := httptest.NewRecorder()
	HandleGrant(rec, req)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc == nil {
		return rec, ""
	}
	return rec, loc.Query().Get("code")
}

func TestHandleGrantBindsResource(t *testing.T) {
	withEncryptionKey(t, "test-key")
	challenge := s256Challenge("verifier-verifier-verifier-123456")
	form := func(resource ...string) url.Values {
		v := url.Values{
			"action": {"approve"}, "client_id": {"test-client"}, "redirect_uri": {"http://localhost/cb"},
			"code_challenge": {challenge}, "code_challenge_method": {"S256"}, "state": {"st"},
		}
		if len(resource) > 0 {
			v["resource"] = resource
		}
		return v
	}

	_, code := grantCode(t, form(testResource+"/"))
	authCode, ok := GetAuthCodeStore().ConsumeAuthCode(code)
	if !ok || authCode.Resource != testResource {
		t.Errorf("code resource = %+v, want %q (canonical)", authCode, testResource)
	}

	_, code = grantCode(t, form())
	if authCode, ok = GetAuthCodeStore().ConsumeAuthCode(code); !ok || authCode.Resource != "" {
		t.Errorf("code resource = %+v, want none", authCode)
	}

	// A forged approval naming another resource gets no code.
	rec, code := grantCode(t, form("https://other.example.com/mcp"))
	if code != "" || rec.Code != http.StatusBadRequest {
		t.Errorf("foreign resource approval = %d, code %q, want 400 and no code", rec.Code, code)
	}
}

func setupTokenDB(t *testing.T) database.DbDriver {
	t.Helper()
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		URL:      "https://knot.example.com",
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })
	service.SetTransport(&gossipCaptureTransport{})
	t.Cleanup(func() { service.SetTransport(nil) })
	return database.GetInstance()
}

func tokenRequest(form url.Values) (*httptest.ResponseRecorder, map[string]any) {
	return postForm(HandleToken, "/token", form)
}

func TestAuthorizationCodeGrantResource(t *testing.T) {
	db := setupTokenDB(t)
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

	exchange := func(bound string, resource ...string) (*httptest.ResponseRecorder, map[string]any) {
		code, err := GetAuthCodeStore().CreateAuthCode("user-1", "client-1", "http://localhost/cb", "mcp", s256Challenge(verifier), "S256", bound)
		if err != nil {
			t.Fatal(err)
		}
		form := url.Values{
			"grant_type": {"authorization_code"}, "code": {code.Code},
			"redirect_uri": {"http://localhost/cb"}, "code_verifier": {verifier},
		}
		if len(resource) > 0 {
			form["resource"] = resource
		}
		return tokenRequest(form)
	}
	audienceOf := func(body map[string]any) string {
		t.Helper()
		id, _ := body["access_token"].(string)
		token, err := db.GetToken(id)
		if err != nil {
			t.Fatalf("issued token not stored: %v (%v)", err, body)
		}
		return token.Audience
	}

	tests := []struct {
		name     string
		bound    string
		resource []string
		wantErr  bool
	}{
		{"bound, same resource", testResource, []string{testResource}, false},
		{"bound, no resource at token", testResource, nil, false},
		{"bound, foreign resource", testResource, []string{"https://other.example.com/mcp"}, true},
		{"unbound, no resource: bound to the MCP endpoint", "", nil, false},
		{"unbound, this server's resource", "", []string{testResource + "/"}, false},
		{"unbound, foreign resource", "", []string{"https://other.example.com/mcp"}, true},
		{"malformed resource", "", []string{"/mcp"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, body := exchange(tt.bound, tt.resource...)
			if tt.wantErr {
				if rec.Code != http.StatusBadRequest || body["error"] != ErrorInvalidTarget {
					t.Errorf("= %d %v, want 400 invalid_target", rec.Code, body)
				}
				return
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("= %d %v, want 200", rec.Code, body)
			}
			if aud := audienceOf(body); aud != testResource {
				t.Errorf("token audience = %q, want %q", aud, testResource)
			}
		})
	}
}

func TestRefreshTokenGrantResource(t *testing.T) {
	db := setupTokenDB(t)

	newToken := func(audience string) *model.Token {
		token := model.NewToken("oauth", "user-1")
		token.RefreshToken = true
		token.Scopes = []string{model.ScopeMCP}
		token.Audience = audience
		if err := db.SaveToken(token); err != nil {
			t.Fatal(err)
		}
		return token
	}
	refresh := func(id string, resource ...string) (*httptest.ResponseRecorder, map[string]any) {
		form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {id}}
		if len(resource) > 0 {
			form["resource"] = resource
		}
		return tokenRequest(form)
	}

	token := newToken(testResource)
	if rec, body := refresh(token.Id, testResource); rec.Code != http.StatusOK {
		t.Errorf("matching resource = %d %v, want 200", rec.Code, body)
	}
	if rec, body := refresh(token.Id, "https://other.example.com/mcp"); rec.Code != http.StatusBadRequest || body["error"] != ErrorInvalidTarget {
		t.Errorf("foreign resource = %d %v, want 400 invalid_target", rec.Code, body)
	}

	// A token from before audiences were recorded gains one on refresh.
	legacy := newToken("")
	if rec, body := refresh(legacy.Id); rec.Code != http.StatusOK {
		t.Fatalf("legacy refresh = %d %v", rec.Code, body)
	}
	if got, _ := db.GetToken(legacy.Id); got == nil || got.Audience != testResource {
		t.Errorf("legacy token audience after refresh = %+v, want %q", got, testResource)
	}
}

// The audience survives the database round trip and the gossip encoding.
func TestTokenAudiencePersists(t *testing.T) {
	db := setupTokenDB(t)

	token := model.NewToken("oauth", "user-1")
	token.Audience = testResource
	if err := db.SaveToken(token); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetToken(token.Id)
	if err != nil || got.Audience != testResource {
		t.Fatalf("GetToken audience = %+v %v, want %q", got, err, testResource)
	}

	// Gossip carries tokens with the cluster's message codec.
	codec := shamaton.New()
	data, err := codec.Marshal([]*model.Token{token})
	if err != nil {
		t.Fatal(err)
	}
	var decoded []*model.Token
	if err := codec.Unmarshal(data, &decoded); err != nil || len(decoded) != 1 || decoded[0].Audience != testResource {
		t.Errorf("gossip round trip = %+v %v, want audience %q", decoded, err, testResource)
	}
}

func TestDeviceAuthorizationResource(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{URL: "https://knot.example.com"})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	rec, body := postForm(HandleDeviceAuthorization, "/device_authorization", url.Values{
		"client_id": {CLIClientId}, "token_name": {"knot client test"},
		"resource": {"https://other.example.com/mcp"},
	})
	if rec.Code != http.StatusBadRequest || body["error"] != ErrorInvalidTarget {
		t.Errorf("foreign resource = %d %v, want 400 invalid_target", rec.Code, body)
	}
}

func contextWithUser(r *http.Request, user *model.User) context.Context {
	return context.WithValue(r.Context(), "user", user)
}
