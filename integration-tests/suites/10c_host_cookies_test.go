//go:build integration

package suites

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/paularlott/knot/integration-tests/harness"
)

// TestHostCookiesBehindTLSProxy runs knot as it runs behind a load
// balancer that ends TLS: its URL is https but it is spoken to over plain
// http, without X-Forwarded-Proto. Its cookies must still take the __Host-
// prefix, so a web server on a wildcard host can't plant a session cookie
// of its own that knot then reads.
func TestHostCookiesBehindTLSProxy(t *testing.T) {
	harness.Feature(t, "vnc-auth")

	lan := harness.LanIP()
	s, err := harness.StartServerAt(cfg, bins, "hostcookies", lan,
		"--wildcard-domain", "*."+lan, "--url", "https://"+lan)
	if err != nil {
		t.Fatalf("boot server: %v", err)
	}
	t.Cleanup(s.Stop)

	// The harness signs in through the API client's session cookie, as
	// `knot connect` does, so this also proves that still works.
	admin, err := harness.ProvisionAdmin(s, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatal(err)
	}
	attacker, err := harness.CreateUser(s, admin, "user1", harness.TesterPermissions())
	if err != nil {
		t.Fatal(err)
	}

	noRedirects := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// webLogin signs in like the browser and returns the session cookie.
	webLogin := func(email, password string) *http.Cookie {
		t.Helper()
		data, _ := json.Marshal(map[string]string{"email": email, "password": password})
		req, _ := http.NewRequest("POST", s.BaseURL+"/api/auth/web", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := noRedirects.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("web login %s = %d", email, resp.StatusCode)
		}
		for _, c := range resp.Cookies() {
			if strings.HasSuffix(c.Name, "__KNOT_WEBUI_SESSION") && c.MaxAge >= 0 {
				return c
			}
		}
		t.Fatalf("web login %s set no session cookie: %v", email, resp.Header.Values("Set-Cookie"))
		return nil
	}

	victim := webLogin("admin@knot.test", "AdminPassw0rd!")
	if victim.Name != "__Host-__KNOT_WEBUI_SESSION" || !victim.Secure || victim.Domain != "" || victim.Path != "/" {
		t.Fatalf("session cookie = %+v, want __Host-, Secure, host-only, Path=/", victim)
	}
	planted := webLogin(attacker.Email, attacker.Password)

	whoAmI := func(cookie string) (string, int) {
		t.Helper()
		req, _ := http.NewRequest("GET", s.BaseURL+"/api/users/whoami", nil)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Cookie", cookie)
		resp, err := noRedirects.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var me struct {
			Username string `json:"username"`
		}
		json.NewDecoder(resp.Body).Decode(&me)
		return me.Username, resp.StatusCode
	}

	// A wildcard host can set a parent-domain cookie only under the plain
	// name; a browser sends it first when its path is longer. knot must
	// ignore it and read only the __Host- one.
	if who, code := whoAmI("__KNOT_WEBUI_SESSION=" + planted.Value + "; __Host-__KNOT_WEBUI_SESSION=" + victim.Value); code != http.StatusOK || who != "admin" {
		t.Errorf("with a planted cookie first, whoami = %d %q, want admin", code, who)
	}
	if who, code := whoAmI("__KNOT_WEBUI_SESSION=" + planted.Value); code != http.StatusUnauthorized {
		t.Errorf("planted cookie alone = %d %q, want 401", code, who)
	}

	// Web VNC's handoff cookie takes the prefix too.
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(s.BaseURL, "http://"))
	vncHost := "admin--box--vnc." + lan + ":" + port
	req, _ := http.NewRequest("GET", s.BaseURL+"/proxy/vnc-login?host="+url.QueryEscape(vncHost), nil)
	req.Header.Set("Cookie", victim.Name+"="+victim.Value)
	resp, err := noRedirects.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	handoff, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusSeeOther || handoff.Scheme != "https" || handoff.Host != vncHost {
		t.Fatalf("vnc-login = %d to %s", resp.StatusCode, handoff)
	}
	// The load balancer ends TLS: reach the VNC host over plain http.
	req, _ = http.NewRequest("GET", s.BaseURL+handoff.RequestURI(), nil)
	req.Host = vncHost
	resp, err = noRedirects.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	var grant *http.Cookie
	for _, c := range resp.Cookies() {
		if strings.HasSuffix(c.Name, "__KNOT_VNC") {
			grant = c
		}
	}
	if grant == nil || grant.Name != "__Host-__KNOT_VNC" || !grant.Secure || grant.Domain != "" {
		t.Fatalf("VNC cookie = %+v (status %d)", grant, resp.StatusCode)
	}
}
