package agenttunnel

import (
	"testing"
	"time"

	"github.com/paularlott/knot/internal/tunnel_server"
)

func newTestClient(name string) *tunnel_server.TunnelClient {
	return tunnel_server.NewTunnelClient("ws://example.invalid", "https://example.invalid", "token", true, &tunnel_server.TunnelOpts{
		Type:       tunnel_server.WebTunnel,
		Protocol:   "http",
		LocalPort:  80,
		TunnelName: name,
	})
}

// A daemon tunnel whose client context dies (e.g. the server closed the
// tunnel) must remove itself from the registry so `tunnel list` does not
// report dead tunnels as running.
func TestRegistryRemovesEntryWhenClientContextDies(t *testing.T) {
	client := newTestClient("cleanup-test")
	if _, ok := Start("cleanup-test", 80, "http", "https://example", client); !ok {
		t.Fatal("expected Start to register the tunnel")
	}
	if !IsTunneled("cleanup-test") {
		t.Fatal("expected tunnel to be registered")
	}

	client.Shutdown()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !IsTunneled("cleanup-test") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("expected registry entry to be removed after the client context died")
}

// The self-cleanup must be identity-checked: a stale client's death cannot
// evict a newer tunnel registered under the same name.
func TestRegistryCleanupDoesNotRemoveReplacement(t *testing.T) {
	oldClient := newTestClient("replace-test")
	if _, ok := Start("replace-test", 80, "http", "https://example", oldClient); !ok {
		t.Fatal("expected Start to register the tunnel")
	}
	if !Stop("replace-test") {
		t.Fatal("expected Stop to remove the tunnel")
	}

	newClient := newTestClient("replace-test")
	if _, ok := Start("replace-test", 8080, "https", "https://example", newClient); !ok {
		t.Fatal("expected Start to re-register the name")
	}

	oldClient.Shutdown()
	time.Sleep(200 * time.Millisecond)

	if !IsTunneled("replace-test") {
		t.Fatal("stale client cleanup removed the replacement tunnel")
	}
	if entry, _ := Get("replace-test"); entry.Port != 8080 {
		t.Fatalf("registry holds the wrong tunnel: port %d, want 8080", entry.Port)
	}

	Stop("replace-test")
}
