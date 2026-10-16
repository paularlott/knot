package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
)

func withServerURL(t *testing.T, serverURL string) {
	t.Helper()
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{URL: serverURL})
	t.Cleanup(func() { config.SetServerConfig(prev) })
}

// Behind a load balancer that ends TLS, knot sees plain http, maybe without
// X-Forwarded-Proto; the https URL still decides the cookie's name and
// Secure flag, so every server in the zone agrees.
func TestCookiesOnHTTPSServer(t *testing.T) {
	withServerURL(t, "https://knot.example.com")

	r := httptest.NewRequest("GET", "http://knot.example.com/", nil) // no TLS, no X-Forwarded-Proto
	w := httptest.NewRecorder()
	SetCookie(w, r, "__KNOT_TEST", "v", 0)
	c := w.Result().Cookies()[0]
	if c.Name != "__Host-__KNOT_TEST" || !c.Secure || c.Domain != "" || c.Path != "/" || !c.HttpOnly {
		t.Errorf("cookie = %+v, want __Host-, Secure, host-only, Path=/, HttpOnly", c)
	}

	// Only the prefixed name is read: one planted under the plain name,
	// even first, is ignored.
	r = httptest.NewRequest("GET", "http://knot.example.com/", nil)
	r.Header.Set("Cookie", "__KNOT_TEST=planted; __Host-__KNOT_TEST=real")
	if got, err := GetCookie(r, "__KNOT_TEST"); err != nil || got.Value != "real" {
		t.Errorf("GetCookie = %v, %v, want real", got, err)
	}
	r.Header.Set("Cookie", "__KNOT_TEST=planted")
	if got, err := GetCookie(r, "__KNOT_TEST"); err == nil {
		t.Errorf("GetCookie read the planted plain cookie: %v", got)
	}

	w = httptest.NewRecorder()
	ExpireCookie(w, r, "__KNOT_TEST")
	names := map[string]bool{}
	for _, c := range w.Result().Cookies() {
		if c.MaxAge >= 0 {
			t.Errorf("%s not expired", c.Name)
		}
		if strings.HasPrefix(c.Name, "__Host-") && !c.Secure {
			t.Errorf("%s deletion isn't Secure, so browsers ignore it", c.Name)
		}
		names[c.Name] = true
	}
	if !names["__Host-__KNOT_TEST"] || !names["__KNOT_TEST"] {
		t.Errorf("expired %v, want both names", names)
	}
}

// Over plain http the prefix can't be used: browsers drop Secure cookies
// there, so the names stay plain and Secure follows the request.
func TestCookiesOnHTTPServer(t *testing.T) {
	withServerURL(t, "http://knot.local:3000")

	r := httptest.NewRequest("GET", "http://knot.local:3000/", nil)
	w := httptest.NewRecorder()
	SetCookie(w, r, "__KNOT_TEST", "v", 600)
	c := w.Result().Cookies()[0]
	if c.Name != "__KNOT_TEST" || c.Secure || c.MaxAge != 600 {
		t.Errorf("cookie = %+v", c)
	}

	r.Header.Set("X-Forwarded-Proto", "https")
	w = httptest.NewRecorder()
	SetCookie(w, r, "__KNOT_TEST", "v", 0)
	if c := w.Result().Cookies()[0]; !c.Secure {
		t.Error("cookie not Secure behind an https proxy")
	}

	r = httptest.NewRequest("GET", "http://knot.local:3000/", nil)
	r.AddCookie(&http.Cookie{Name: "__KNOT_TEST", Value: "v"})
	if got, err := GetCookie(r, "__KNOT_TEST"); err != nil || got.Value != "v" {
		t.Errorf("GetCookie = %v, %v", got, err)
	}
}
