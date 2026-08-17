//go:build integration

package suites

import (
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration/harness"
)

func TestPortForwarding(t *testing.T) {
	harness.Feature(t, "port-forwarding")

	// Forward a local port in the source space to the sshd of the shared
	// workhorse space.
	to := workspace(t)
	from := spaceFixture(t, "it-pf-from", user1.Id, user1.Client)

	ctx, cancel := testCtx(120)
	defer cancel()

	// Forward local 8022 in `from` to port 2222 (sshd) in the workhorse.
	if code, err := user1.Client.ForwardPort(ctx, from, &apiclient.PortForwardRequest{
		LocalPort:  8022,
		Space:      "it-wh",
		RemotePort: 2222,
	}); err != nil {
		t.Fatalf("forward port: %v (status %d)", err, code)
	}
	t.Cleanup(func() {
		ctx, cancel := testCtx(30)
		user1.Client.StopPort(ctx, from, &apiclient.PortStopRequest{LocalPort: 8022})
		cancel()
	})

	// The forward appears in the list.
	var fwd *apiclient.PortForwardInfo
	deadlineSeconds := 60
	waitFor(t, deadlineSeconds, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		ports, _, err := user1.Client.ListPorts(ctx, from)
		if err != nil {
			return false
		}
		for i := range ports.Forwards {
			if ports.Forwards[i].LocalPort == 8022 {
				fwd = &ports.Forwards[i]
				return true
			}
		}
		return false
	})
	if fwd == nil {
		t.Fatal("port forward not listed")
	}
	if fwd.Space != "it-wh" || fwd.RemotePort != 2222 {
		t.Fatalf("forward info = %+v", fwd)
	}

	// Data flows through it: the sshd banner is readable via /dev/tcp.
	out := harness.RunCommand(t, user1.Client, from, 60,
		"timeout 10 bash -c 'exec 3<>/dev/tcp/127.0.0.1/8022; head -c 16 <&3' || true")
	mustContain(t, "sshd banner via forward", out, "SSH")
	_ = to

	// Throttle the forward.
	if code, err := user1.Client.ThrottlePort(ctx, from, &apiclient.PortThrottleRequest{
		LocalPort: 8022, LatencyMs: 50, JitterMs: 5, BandwidthKB: 512,
	}); err != nil {
		t.Fatalf("throttle port: %v (status %d)", err, code)
	}
	ports, _, err := user1.Client.ListPorts(ctx, from)
	if err != nil {
		t.Fatalf("list ports after throttle: %v", err)
	}
	throttled := false
	for _, f := range ports.Forwards {
		if f.LocalPort == 8022 && f.LatencyMs == 50 {
			throttled = true
		}
	}
	if !throttled {
		t.Fatalf("throttle not reflected in port list: %+v", ports.Forwards)
	}

	// Stop the forward.
	if code, err := user1.Client.StopPort(ctx, from, &apiclient.PortStopRequest{LocalPort: 8022}); err != nil {
		t.Fatalf("stop port: %v (status %d)", err, code)
	}
	ports, _, _ = user1.Client.ListPorts(ctx, from)
	for _, f := range ports.Forwards {
		if f.LocalPort == 8022 {
			t.Fatalf("forward still listed after stop: %+v", f)
		}
	}
}
