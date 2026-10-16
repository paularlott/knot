//go:build integration

package suites

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

func TestSpaceTunnels(t *testing.T) {
	harness.Feature(t, "tunnels")
	if cfg.Runtime != "docker" {
		t.Skip("tunnel test needs docker networking")
	}

	s, adminUser := bootDedicated(t, "tunnel",
		"--listen-tunnel", "0.0.0.0:18322",
		"--tunnel-domain", "tunnel.knot.test",
	)

	// The dedicated server has its own database; create its template.
	tunnelTemplate, err := harness.CreateTemplate(s, adminUser.Client, uniqueName("it_tunnel_tpl"), harness.TemplateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	id := harness.CreateSpace(t, adminUser.Client, "it-tunnel", tunnelTemplate, adminUser.Id)
	harness.DeleteSpaceAsync(t, adminUser.Client, id)
	harness.WaitForSpaceReady(t, s, adminUser.Client, id)

	ctx, cancel := testCtx(60)
	defer cancel()

	// Server tunnel info is available.
	info, code, err := adminUser.Client.GetTunnelServerInfo(ctx)
	if err != nil {
		t.Fatalf("tunnel server info: %v (status %d)", err, code)
	}
	if info == nil {
		t.Fatal("nil tunnel server info")
	}

	// Start an http tunnel exposing an http server in the space that
	// answers with the Cookie header it was sent.
	harness.RunCommand(t, adminUser.Client, id, 30,
		"echo aW1wb3J0IGh0dHAuc2VydmVyCgoKY2xhc3MgRWNobyhodHRwLnNlcnZlci5CYXNlSFRUUFJlcXVlc3RIYW5kbGVyKToKICAgIGRlZiBkb19HRVQoc2VsZik6CiAgICAgICAgYm9keSA9ICgiY29va2llPSIgKyAoc2VsZi5oZWFkZXJzLmdldCgiQ29va2llIikgb3IgIiIpKS5lbmNvZGUoKQogICAgICAgIHNlbGYuc2VuZF9yZXNwb25zZSgyMDApCiAgICAgICAgc2VsZi5zZW5kX2hlYWRlcigiQ29udGVudC1MZW5ndGgiLCBzdHIobGVuKGJvZHkpKSkKICAgICAgICBzZWxmLmVuZF9oZWFkZXJzKCkKICAgICAgICBzZWxmLndmaWxlLndyaXRlKGJvZHkpCgogICAgZGVmIGxvZ19tZXNzYWdlKHNlbGYsICphcmdzKToKICAgICAgICBwYXNzCgoKaHR0cC5zZXJ2ZXIuSFRUUFNlcnZlcigoIjAuMC4wLjAiLCA4MDgwKSwgRWNobykuc2VydmVfZm9yZXZlcigpCg== | base64 -d > /tmp/it-echo.py")
	harness.RunCommand(t, adminUser.Client, id, 30,
		"nohup python3 /tmp/it-echo.py >/dev/null 2>&1 &")
	if !waitForCond(90, func() bool {
		out, err := harness.TryRunCommand(adminUser.Client, id, 20,
			"curl -s -o /dev/null -w %{http_code} http://127.0.0.1:8080/ || true")
		return err == nil && strings.TrimSpace(out) == "200"
	}) {
		t.Fatal("in-space http server never came up")
	}
	resp, code, err := adminUser.Client.StartSpaceTunnel(ctx, id, &apiclient.SpaceTunnelStartRequest{
		Protocol: "http",
		Port:     8080,
		Name:     "it-tunnel-test",
	})
	if err != nil {
		t.Fatalf("start space tunnel: %v (status %d)", err, code)
	}
	t.Cleanup(func() {
		ctx, cancel := testCtx(30)
		adminUser.Client.StopSpaceTunnel(ctx, id, &apiclient.SpaceTunnelStopRequest{Name: "it-tunnel-test"})
		cancel()
	})
	if resp == nil {
		t.Fatal("nil tunnel start response")
	}

	// The tunnel appears in the space's tunnel list.
	waitFor(t, 30, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		tunnels, _, err := adminUser.Client.ListSpaceTunnels(ctx, id)
		if err != nil || tunnels == nil {
			return false
		}
		for _, tn := range tunnels.Tunnels {
			if tn.Name == "it-tunnel-test" {
				return true
			}
		}
		return false
	})

	// Traffic flows through the tunnel, and the service behind it never
	// sees knot's own cookies, prefixed or not; its own pass through.
	tunnelURL, err := url.Parse(resp.URL)
	if err != nil || tunnelURL.Hostname() == "" {
		t.Fatalf("tunnel URL %q has no hostname", resp.URL)
	}
	body, status := "", 0
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(time.Second) {
		if body, status = tunnelGet(tunnelURL.Hostname(), "app=1; __KNOT_WEBUI_SESSION=s; __Host-__KNOT_WEBUI_SESSION=h; __KNOT_VNC=v"); status == http.StatusOK {
			break
		}
	}
	if status != http.StatusOK {
		t.Fatalf("request through the tunnel = %d", status)
	}
	if body != "cookie=app=1" {
		t.Errorf("service behind the tunnel saw %q, want only its own cookie", body)
	}

	// Stop it.
	if code, err := adminUser.Client.StopSpaceTunnel(ctx, id, &apiclient.SpaceTunnelStopRequest{
		Name: "it-tunnel-test",
	}); err != nil {
		t.Fatalf("stop space tunnel: %v (status %d)", err, code)
	}

	_ = apiclient.SpaceTunnelStartRequest{}
}

// tunnelGet makes one request through the tunnel listener, addressed by the
// tunnel's hostname, and returns the body and status.
func tunnelGet(tunnelHost, cookie string) (string, int) {
	req, err := http.NewRequest("GET", "http://127.0.0.1:18322/", nil)
	if err != nil {
		return "", 0
	}
	req.Host = tunnelHost
	req.Header.Set("Cookie", cookie)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", 0
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), resp.StatusCode
}
