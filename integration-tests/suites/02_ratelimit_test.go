//go:build integration

package suites

import (
	"net/http"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// TestAuthRateLimitAndFlush runs on the shared server: exhaust the failed
// login allowance, verify even a correct password is blocked, then flush the
// blocks through the admin API and log in again. The flush replaces a server
// restart (state is in-memory only).
func TestAuthRateLimitAndFlush(t *testing.T) {
	harness.Feature(t, "auth-rate-limiting")

	c, _ := apiclient.NewClient(server.BaseURL, "", true)
	c.SetContentType("application/json")
	c.GetRESTClient().SetAccept("application/json")

	// Exhaust the allowance (default server: 8 failures / 300s window).
	ctx, cancel := testCtx(60)
	defer cancel()
	for i := 0; i < 8; i++ {
		_, code, err := c.Login(ctx, "admin@knot.test", "definitely-wrong", "")
		if err == nil || code != http.StatusUnauthorized {
			t.Fatalf("bad login %d: code=%d err=%v", i, code, err)
		}
	}

	// Even the correct password is blocked now.
	_, code, err := c.Login(ctx, "admin@knot.test", "AdminPassw0rd!", "")
	if err == nil {
		t.Fatal("login succeeded while rate limited")
	}
	if code != http.StatusTooManyRequests && code != http.StatusUnauthorized && code != http.StatusForbidden {
		t.Fatalf("blocked login status = %d, want 429/401/403", code)
	}

	// The flush API clears the blocks.
	if err := admin.Client.ClearAuthBlocks(ctx); err != nil {
		t.Fatalf("clear auth blocks: %v", err)
	}

	resp, code, err := c.Login(ctx, "admin@knot.test", "AdminPassw0rd!", "")
	if err != nil {
		t.Fatalf("login after flush: %v (status %d)", err, code)
	}
	if resp.Token == "" {
		t.Fatal("no token after flush")
	}
}

// TestServerRestartPreservesState restarts the shared server: everything in
// badger (users, tokens) survives and in-memory blocks are gone.
func TestServerRestartPreservesState(t *testing.T) {
	harness.Feature(t, "server-info")
	if testing.Short() {
		t.Skip("short mode")
	}

	// Identity before restart.
	ctx, cancel := testCtx(30)
	defer cancel()
	who, err := admin.Client.WhoAmI(ctx)
	if err != nil {
		t.Fatalf("whoami before restart: %v", err)
	}
	mustEqual(t, "username before restart", who.Username, "admin")

	if err := server.Restart(); err != nil {
		t.Fatalf("restart server: %v", err)
	}

	// API tokens survive the restart.
	who, err = admin.Client.WhoAmI(ctx)
	if err != nil {
		t.Fatalf("whoami after restart: %v", err)
	}
	mustEqual(t, "username after restart", who.Username, "admin")

	// And user1's token too.
	who, err = user1.Client.WhoAmI(ctx)
	if err != nil {
		t.Fatalf("user1 whoami after restart: %v", err)
	}
	mustEqual(t, "user1 after restart", who.Username, "user1")
}
