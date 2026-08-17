//go:build integration

package suites

import (
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration/harness"
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

	// Start an http tunnel exposing the space's http server.
	harness.RunCommand(t, adminUser.Client, id, 30,
		"nohup python3 -m http.server 8080 >/dev/null 2>&1 &")
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

	// Stop it.
	if code, err := adminUser.Client.StopSpaceTunnel(ctx, id, &apiclient.SpaceTunnelStopRequest{
		Name: "it-tunnel-test",
	}); err != nil {
		t.Fatalf("stop space tunnel: %v (status %d)", err, code)
	}

	_ = apiclient.SpaceTunnelStartRequest{}
}
