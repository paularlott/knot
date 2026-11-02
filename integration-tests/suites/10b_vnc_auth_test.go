//go:build integration

package suites

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/integration-tests/harness"
)

// TestWebVNCSignIn proves knot's session cookie stays on knot's own host
// and web VNC hosts sign in with their own cookie through a single-use
// handoff, so the users' code behind wildcard hosts never gets a session.
func TestWebVNCSignIn(t *testing.T) {
	harness.Feature(t, "vnc-auth")

	// The wildcard mux routes by Host, so the server's URL host must be
	// what the harness dials: the LAN address. The wildcard domain sits
	// above it, as in a deployment where knot.example.com serves
	// *.knot.example.com: there, earlier versions shared the session
	// cookie with every wildcard host.
	lan := harness.LanIP()
	domain := lan
	s, err := harness.StartServerAt(cfg, bins, "vnc", lan, "--wildcard-domain", "*."+domain)
	if err != nil {
		t.Fatalf("boot vnc server: %v", err)
	}
	t.Cleanup(s.Stop)
	if _, err := harness.ProvisionAdmin(s, "admin", "AdminPassw0rd!"); err != nil {
		t.Fatal(err)
	}

	// A browser that resolves every wildcard host to the server and
	// doesn't follow redirects, so each hop is checked.
	serverAddr := strings.TrimPrefix(s.BaseURL, "http://")
	newBrowser := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		return &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
			Transport: &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if host, _, _ := net.SplitHostPort(addr); strings.HasSuffix(host, "."+domain) {
					addr = serverAddr
				}
				return dialer.DialContext(ctx, network, addr)
			}},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	get := func(b *http.Client, u string) *http.Response {
		t.Helper()
		resp, err := b.Get(u)
		if err != nil {
			t.Fatalf("GET %s: %v", u, err)
		}
		resp.Body.Close()
		return resp
	}

	browser := newBrowser()
	data, _ := json.Marshal(map[string]string{"email": "admin@knot.test", "password": "AdminPassw0rd!"})
	req, _ := http.NewRequest("POST", s.BaseURL+"/api/auth/web", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := browser.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("web login = %d", resp.StatusCode)
	}

	// The session cookie is host-only: no Domain attribute, so the
	// browser never sends it to a wildcard host.
	sawSession := false
	for _, line := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(line, "__KNOT_WEBUI_SESSION=") && !strings.Contains(line, "Max-Age=0") {
			sawSession = true
			if strings.Contains(strings.ToLower(line), "domain=") {
				t.Errorf("session cookie isn't host-only: %s", line)
			}
		}
	}
	if !sawSession {
		t.Fatal("login set no session cookie")
	}

	_, port, _ := net.SplitHostPort(serverAddr)
	vncHost := "admin--box--vnc." + domain + ":" + port
	vncURL := "http://" + vncHost

	// 1. Without its cookie the VNC host sends the browser to sign in.
	resp = get(browser, vncURL+"/vnc.html?autoconnect=1")
	loginURL, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusSeeOther || loginURL.Path != "/proxy/vnc-login" || loginURL.Query().Get("host") != vncHost {
		t.Fatalf("VNC host without a cookie = %d to %s", resp.StatusCode, loginURL)
	}

	// 2. knot's host hands back a code for this VNC host.
	resp = get(browser, loginURL.String())
	handoff, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusSeeOther || handoff.Host != vncHost || handoff.Path != "/.knot-auth/vnc" || handoff.Query().Get("code") == "" {
		t.Fatalf("vnc-login = %d to %s", resp.StatusCode, handoff)
	}
	if resp.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("vnc-login Referrer-Policy = %q", resp.Header.Get("Referrer-Policy"))
	}

	// A code works on its own host only, so try another host first.
	other := *handoff
	other.Host = "admin--other--vnc." + domain + ":" + port
	if resp := get(newBrowser(), other.String()); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("code on another VNC host = %d, want 400", resp.StatusCode)
	}

	// 3. The VNC host trades the code for its own host-only cookie.
	resp = get(browser, handoff.String())
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/vnc.html?autoconnect=1" {
		t.Fatalf("handoff = %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	sawGrant := false
	for _, line := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(line, "__KNOT_VNC=") {
			sawGrant = true
			lower := strings.ToLower(line)
			if strings.Contains(lower, "domain=") || !strings.Contains(lower, "httponly") {
				t.Errorf("VNC cookie must be host-only and HttpOnly: %s", line)
			}
		}
	}
	if !sawGrant {
		t.Fatal("handoff set no VNC cookie")
	}

	// Each code works once.
	if resp := get(newBrowser(), handoff.String()); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("replayed code = %d, want 400", resp.StatusCode)
	}

	// Signed in: the request reaches the VNC proxy, which has no space
	// called "box" to show, rather than being sent to sign in.
	resp = get(browser, vncURL+"/vnc.html?autoconnect=1")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("VNC host with its cookie = %d (Location %q), want 404 from the proxy", resp.StatusCode, resp.Header.Get("Location"))
	}

	// Signing out of knot ends VNC access too.
	req, _ = http.NewRequest("POST", s.BaseURL+"/api/auth/logout", nil)
	req.Header.Set("Accept", "application/json")
	if resp, err := browser.Do(req); err != nil || resp.StatusCode >= 300 {
		t.Fatalf("logout: %v %v", resp, err)
	} else {
		resp.Body.Close()
	}
	resp = get(browser, vncURL+"/vnc.html")
	if resp.StatusCode != http.StatusSeeOther || !strings.Contains(resp.Header.Get("Location"), "/proxy/vnc-login") {
		t.Errorf("VNC host after logout = %d to %q, want a redirect to sign in", resp.StatusCode, resp.Header.Get("Location"))
	}

	// And vnc-login itself needs a signed-in browser.
	resp = get(newBrowser(), loginURL.String())
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/login") {
		t.Errorf("vnc-login signed out = %d to %q, want the login page", resp.StatusCode, resp.Header.Get("Location"))
	}
}
