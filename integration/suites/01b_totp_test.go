//go:build integration

package suites

import (
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration/harness"
	"github.com/paularlott/knot/internal/totp"
)

// awaitStableSlice waits until well past any 30s TOTP boundary so codes
// computed for now/now-1/now+2 don't race a rollover mid-request.
func awaitStableSlice(t *testing.T) int64 {
	t.Helper()
	for {
		now := time.Now().UTC().Unix()
		into := now % 30
		if into >= 5 && into <= 20 {
			return now / 30
		}
		if into < 5 {
			time.Sleep(time.Duration(6-into) * time.Second)
		} else {
			time.Sleep(time.Duration(31-into) * time.Second)
		}
	}
}

func TestTOTPSecondFactor(t *testing.T) {
	harness.Feature(t, "totp")
	ctx, cancel := testCtx(90)
	defer cancel()

	// The shared journey server runs with TOTP disabled.
	using, _, err := user1.Client.UsingTOTP(ctx)
	if err != nil {
		t.Fatalf("using-totp on shared server: %v", err)
	}
	mustEqual(t, "totp disabled on shared server", using, false)

	// Dedicated server with TOTP on and a 1-step (±30s) acceptance window.
	s, adminUser := bootDedicated(t, "totp", "--enable-totp", "--totp-window", "1")

	using, _, err = adminUser.Client.UsingTOTP(ctx)
	if err != nil {
		t.Fatalf("using-totp on dedicated server: %v", err)
	}
	mustEqual(t, "totp enabled on dedicated server", using, true)

	// A fresh user; their first login mints a secret and reveals it once.
	name := uniqueName("it-totp")
	password := "Passw0rd!totp"
	email := name + "@knot.test"
	userId, code, err := adminUser.Client.CreateUser(ctx, &apiclient.CreateUserRequest{
		Username: name, Password: password, Email: email,
		Roles: []string{}, Active: true,
		MaxSpaces: 1, ComputeUnits: 1, StorageUnits: 1, MaxTunnels: 1,
		PreferredShell: "bash", Timezone: "UTC",
	})
	if err != nil {
		t.Fatalf("create totp user: %v (status %d)", err, code)
	}

	anon := harness.NewAnonClient(s)
	resp, code, err := anon.Login(ctx, email, password, "")
	if err != nil {
		t.Fatalf("first login should mint a secret without a code: %v (status %d)", err, code)
	}
	if resp.TOTPSecret == "" {
		t.Fatal("first login did not reveal the TOTP secret")
	}
	secret := resp.TOTPSecret

	// The admin can see the minted secret on the user record.
	user, err := adminUser.Client.GetUser(ctx, userId)
	if err != nil {
		t.Fatalf("get totp user: %v", err)
	}
	mustEqual(t, "stored secret", user.TOTPSecret, secret)

	awaitStableSlice(t)

	// Password alone no longer works.
	if _, code, err := anon.Login(ctx, email, password, ""); err == nil {
		t.Fatal("login without TOTP code succeeded")
	} else {
		mustEqual(t, "missing code status", code, 401)
	}

	// A wrong code is refused.
	bad := "000000"
	if c, err := totp.GetCode(secret, 0); err == nil && c == bad {
		bad = "999999"
	}
	if _, code, err := anon.Login(ctx, email, password, bad); err == nil {
		t.Fatal("login with wrong TOTP code succeeded")
	} else {
		mustEqual(t, "wrong code status", code, 401)
	}

	// The correct code authenticates and yields a working session.
	slice := awaitStableSlice(t)
	good, err := totp.GetCode(secret, slice)
	if err != nil {
		t.Fatalf("compute code: %v", err)
	}
	resp, code, err = anon.Login(ctx, email, password, good)
	if err != nil {
		t.Fatalf("login with correct code: %v (status %d)", err, code)
	}
	if resp.Token == "" {
		t.Fatal("TOTP login returned no session token")
	}
	anon.SetAuthToken(resp.Token)
	who, err := anon.WhoAmI(ctx)
	if err != nil {
		t.Fatalf("whoami with totp session: %v", err)
	}
	mustEqual(t, "whoami username", who.Username, name)

	// The ±1 step window accepts the previous slice's code...
	prev, err := totp.GetCode(secret, slice-1)
	if err != nil {
		t.Fatalf("compute previous code: %v", err)
	}
	if _, code, err := anon.Login(ctx, email, password, prev); err != nil {
		t.Fatalf("login with previous-slice code (window 1): %v (status %d)", err, code)
	}

	// ...but two steps ahead is outside the window.
	future, err := totp.GetCode(secret, slice+2)
	if err != nil {
		t.Fatalf("compute future code: %v", err)
	}
	if _, code, err := anon.Login(ctx, email, password, future); err == nil {
		t.Fatal("login with code 2 steps ahead succeeded")
	} else {
		mustEqual(t, "future code status", code, 401)
	}

	// Admin reset (the API path `knot admin reset-totp` uses): clearing the
	// secret makes the next login mint and reveal a fresh one, no code needed.
	info, err := adminUser.Client.GetUser(ctx, userId)
	if err != nil {
		t.Fatalf("get user before reset: %v", err)
	}
	if err := adminUser.Client.UpdateUser(ctx, userId, &apiclient.UpdateUserRequest{
		Username: info.Username, Email: info.Email,
		Roles: info.Roles, Groups: info.Groups, Active: info.Active,
		MaxSpaces: info.MaxSpaces, ComputeUnits: info.ComputeUnits,
		StorageUnits: info.StorageUnits, MaxTunnels: info.MaxTunnels,
		PreferredShell: info.PreferredShell, Timezone: info.Timezone,
		TOTPSecret: "",
	}); err != nil {
		t.Fatalf("reset totp: %v", err)
	}
	resp, code, err = anon.Login(ctx, email, password, "")
	if err != nil {
		t.Fatalf("login after reset should not need a code: %v (status %d)", err, code)
	}
	if resp.TOTPSecret == "" || resp.TOTPSecret == secret {
		t.Fatalf("reset did not mint a fresh secret (old=%q new=%q)", secret, resp.TOTPSecret)
	}
}
