//go:build integration

package suites

import (
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration/harness"
)

func TestLoginWrongPassword(t *testing.T) {
	harness.Feature(t, "auth")
	c, err := apiclient.NewClient(server.BaseURL, "", true)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	c.SetContentType("application/json")
	ctx, cancel := testCtx(15)
	defer cancel()
	_, code, err := c.Login(ctx, "admin@knot.test", "wrong-password", "")
	if err == nil {
		t.Fatal("login with wrong password unexpectedly succeeded")
	}
	mustEqual(t, "login status", code, 401)
}

func TestLoginLogout(t *testing.T) {
	harness.Feature(t, "auth")
	c, err := apiclient.NewClient(server.BaseURL, "", true)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	c.SetContentType("application/json")
	c.UseSessionCookie(true)

	ctx, cancel := testCtx(15)
	defer cancel()
	resp, code, err := c.Login(ctx, "admin@knot.test", "AdminPassw0rd!", "")
	if err != nil {
		t.Fatalf("login: %v (status %d)", err, code)
	}
	if resp.Token == "" {
		t.Fatal("login returned no session token")
	}
	c.SetAuthToken(resp.Token)

	who, err := c.WhoAmI(ctx)
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	mustEqual(t, "whoami username", who.Username, "admin")

	if err := c.Logout(ctx); err != nil {
		t.Fatalf("logout: %v", err)
	}

	// After logout the session cookie is dead (allow a short propagation
	// window for the session store).
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := c.WhoAmI(ctx); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("whoami after logout unexpectedly succeeded")
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func TestTokenAuthAndScopes(t *testing.T) {
	harness.Feature(t, "tokens")
	ctx, cancel := testCtx(20)
	defer cancel()

	// Full-scope token can hit any endpoint.
	who, err := admin.Client.WhoAmI(ctx)
	if err != nil {
		t.Fatalf("whoami with admin token: %v", err)
	}
	mustEqual(t, "admin username", who.Username, "admin")

	// Scoped token is restricted to its scope.
	scoped, code, err := admin.Client.CreateToken(ctx, "scoped", []string{"methods"})
	if err != nil {
		t.Fatalf("create scoped token: %v (status %d)", err, code)
	}
	sc := harness.NewClient(server, scoped)
	if _, err := sc.WhoAmI(ctx); err == nil {
		t.Fatal("scoped token reached /api/users — scope not enforced")
	}
	if _, err := sc.GetMethods(ctx); err != nil {
		t.Fatalf("scoped token should reach methods: %v", err)
	}

	// Deleting a token revokes it immediately.
	plain, code, err := admin.Client.CreateToken(ctx, "to-delete", nil)
	if err != nil {
		t.Fatalf("create token: %v (status %d)", err, code)
	}
	tc := harness.NewClient(server, plain)
	if _, err := tc.WhoAmI(ctx); err != nil {
		t.Fatalf("whoami with new token: %v", err)
	}
	tokens, _, err := admin.Client.GetTokens(ctx)
	if err != nil {
		t.Fatalf("list tokens: %v", err)
	}
	var id string
	for _, tok := range *tokens {
		if tok.Name == "to-delete" {
			id = tok.Id
		}
	}
	if id == "" {
		t.Fatal("created token not in list")
	}
	if code, err := admin.Client.DeleteToken(ctx, id); err != nil {
		t.Fatalf("delete token: %v (status %d)", err, code)
	}
	if _, err := tc.WhoAmI(ctx); err == nil {
		t.Fatal("deleted token still authenticates")
	}
}
