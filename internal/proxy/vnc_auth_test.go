package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/crypt"
)

func withVNCConfig(t *testing.T) *config.ServerConfig {
	t.Helper()
	prev := config.GetServerConfig()
	cfg := &config.ServerConfig{
		URL:            "https://knot.example.com",
		WildcardDomain: "*.knot.example.com",
		EncryptionKey:  crypt.CreateKey(),
	}
	config.SetServerConfig(cfg)
	t.Cleanup(func() { config.SetServerConfig(prev) })
	return cfg
}

func TestIsWildcardVNCHost(t *testing.T) {
	cfg := withVNCConfig(t)
	for host, want := range map[string]bool{
		"alice--box--vnc.knot.example.com":          true,
		"alice--box--vnc.knot.example.com:8443":     true,
		"alice--box--8080.knot.example.com":         false,
		"alice--box--vnc.evil.example.com":          false,
		"alice--box--vnc.x.knot.example.com":        false,
		"alice--box--vnc.knot.example.com.evil":     false,
		"evil.com/alice--box--vnc.knot.example.com": false,
		"knot.example.com":                          false,
		"":                                          false,
	} {
		if got := isWildcardVNCHost(cfg, host); got != want {
			t.Errorf("isWildcardVNCHost(%q) = %v, want %v", host, got, want)
		}
	}
}

// Without its cookie, a VNC host sends a browser to sign in on knot's
// host, and refuses anything else.
func TestVNCHostWithoutCookie(t *testing.T) {
	withVNCConfig(t)
	host := "alice--box--vnc.knot.example.com"

	r := httptest.NewRequest("GET", "https://"+host+"/vnc.html?autoconnect=1", nil)
	r.Host = host
	r.AddCookie(&http.Cookie{Name: model.WebSessionCookie, Value: "a-real-looking-session"})
	w := httptest.NewRecorder()
	handleVNCHost(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("GET = %d, want 303", w.Code)
	}
	loc, _ := url.Parse(w.Header().Get("Location"))
	if loc.Host != "knot.example.com" || loc.Path != "/proxy/vnc-login" ||
		loc.Query().Get("host") != host || loc.Query().Get("return") != "/vnc.html?autoconnect=1" {
		t.Errorf("Location = %s", loc)
	}

	ws := httptest.NewRequest("GET", "https://"+host+"/websockify", nil)
	ws.Host = host
	ws.Header.Set("Upgrade", "websocket")
	w = httptest.NewRecorder()
	handleVNCHost(w, ws)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("websocket = %d, want 401", w.Code)
	}

	post := httptest.NewRequest("POST", "https://"+host+"/", nil)
	post.Host = host
	w = httptest.NewRecorder()
	handleVNCHost(w, post)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("POST = %d, want 401", w.Code)
	}

	// A grant for another VNC host doesn't count.
	other, _ := crypt.SealVNCGrant(config.GetServerConfig().EncryptionKey, "bob--box--vnc.knot.example.com",
		crypt.VNCGrant{Kind: crypt.VNCGrantCookie, SessionId: "s"})
	r = httptest.NewRequest("GET", "https://"+host+"/", nil)
	r.Host = host
	r.AddCookie(&http.Cookie{Name: vncGrantCookie, Value: other})
	w = httptest.NewRecorder()
	handleVNCHost(w, r)
	if w.Code != http.StatusSeeOther {
		t.Errorf("grant for another host = %d, want a redirect to sign in", w.Code)
	}
}

func TestVNCLogin(t *testing.T) {
	cfg := withVNCConfig(t)
	host := "alice--box--vnc.knot.example.com"
	session := &model.Session{Id: "session-1"}

	login := func(query string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "https://knot.example.com/proxy/vnc-login?"+query, nil)
		r = r.WithContext(context.WithValue(r.Context(), "session", session))
		w := httptest.NewRecorder()
		HandleVNCLogin(w, r)
		return w
	}

	w := login(url.Values{"host": {host}, "return": {"/vnc.html"}}.Encode())
	if w.Code != http.StatusSeeOther || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("login = %d %v", w.Code, w.Header())
	}
	loc, _ := url.Parse(w.Header().Get("Location"))
	if loc.Scheme != "https" || loc.Host != host || loc.Path != vncAuthPath || loc.Query().Get("return") != "/vnc.html" {
		t.Fatalf("Location = %s", loc)
	}
	g, ok := crypt.OpenVNCGrant(cfg.EncryptionKey, host, loc.Query().Get("code"))
	if !ok || g.Kind != crypt.VNCGrantHandoff || g.SessionId != "session-1" {
		t.Fatalf("code = %+v, %v", g, ok)
	}
	if left := time.Until(time.Unix(g.Expires, 0)); left <= 0 || left > vncHandoffLife {
		t.Errorf("code lasts %v", left)
	}

	for _, bad := range []string{
		"host=evil.example.com",
		"host=alice--box--8080.knot.example.com",
		"host=" + url.QueryEscape("evil.com/x--y--vnc.knot.example.com"),
		"",
	} {
		if w := login(bad); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, w.Code)
		}
	}

	// An open redirect in return is dropped, not followed.
	w = login(url.Values{"host": {host}, "return": {"//evil.com/x"}}.Encode())
	loc, _ = url.Parse(w.Header().Get("Location"))
	if strings.Contains(loc.Query().Get("return"), "evil") {
		t.Errorf("return = %q", loc.Query().Get("return"))
	}
}
