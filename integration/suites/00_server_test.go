//go:build integration

package suites

import (
	"testing"

	"github.com/paularlott/knot/integration/harness"
)

func TestHealthEndpoint(t *testing.T) {
	harness.Feature(t, "server-info")
	resp, err := rawGet(server.BaseURL+"/health", "")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer resp.Body.Close()
	mustEqual(t, "health status", resp.StatusCode, 200)
}

func TestPing(t *testing.T) {
	harness.Feature(t, "server-info")
	ctx, cancel := testCtx(15)
	defer cancel()
	ping, err := admin.Client.Ping(ctx)
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if ping == nil {
		t.Fatal("nil ping response")
	}
}

func TestServerInfo(t *testing.T) {
	harness.Feature(t, "server-info")
	ctx, cancel := testCtx(15)
	defer cancel()
	info, code, err := admin.Client.GetServerInfo(ctx)
	if err != nil {
		t.Fatalf("server info: %v (status %d)", err, code)
	}
	if info == nil {
		t.Fatal("nil server info")
	}
}

func TestClusterInfo(t *testing.T) {
	harness.Feature(t, "server-info")
	// The local node registers itself with the gossip cluster shortly
	// after boot; poll for it.
	waitFor(t, 30, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		nodes, code, err := admin.Client.GetClusterInfo(ctx)
		return err == nil && code == 200 && len(*nodes) > 0
	})
	ctx, cancel := testCtx(15)
	defer cancel()
	nodes, code, err := admin.Client.GetClusterInfo(ctx)
	if err != nil {
		t.Fatalf("cluster info: %v (status %d)", err, code)
	}
	if len(*nodes) == 0 {
		t.Fatal("no cluster nodes reported (local node never registered)")
	}
}
