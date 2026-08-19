package specwizard

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
)

// TestFetchDecision locks the fetch rule. Notation: M = manifest file set,
// A = update-enabled on, U = update-url set. Startup and manual use the same
// decision: fetch iff A && (!M || U); URL is the configured url when set, else
// the default (only reachable when !M).
func TestFetchDecision(t *testing.T) {
	cfg := func(m, a, u bool) *config.ServerConfig {
		c := &config.ServerConfig{BaseImagesUpdateEnabled: a}
		if m {
			c.BaseImagesManifest = "/x.toml"
		}
		if u {
			c.BaseImagesUpdateURL = "https://example.com/given.toml"
		}
		return c
	}

	cases := []struct {
		name        string
		m, a, u     bool
		wantOK      bool
		wantDefault bool // true → expect DefaultUpdateURL
	}{
		// Gate off → never fetch, regardless of M/U.
		{"A=F !M !U", false, false, false, false, false},
		{"A=F !M  U", false, false, true, false, false},
		{"A=F  M !U", true, false, false, false, false},
		{"A=F  M  U", true, false, true, false, false},

		// Gate on, no file → always fetch (default URL when none given).
		{"A=T !M !U", false, true, false, true, true},
		{"A=T !M  U", false, true, true, true, false},

		// Gate on, file set → fetch only when an explicit URL is given.
		{"A=T  M !U", true, true, false, false, false},
		{"A=T  M  U", true, true, true, true, false},
	}
	for _, c := range cases {
		url, ok := FetchDecision(cfg(c.m, c.a, c.u))
		if ok != c.wantOK {
			t.Errorf("%s: got ok=%v want %v", c.name, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if c.wantDefault && url != DefaultUpdateURL {
			t.Errorf("%s: got url %q, want default %q", c.name, url, DefaultUpdateURL)
		}
		if !c.wantDefault && url == DefaultUpdateURL {
			t.Errorf("%s: got default url, want the configured url", c.name)
		}
	}

	if _, ok := FetchDecision(nil); ok {
		t.Error("nil cfg should never fetch")
	}
}

// TestFetchDisabledReason checks the explanatory message for the no-fetch
// cases, since the refresh API surfaces it.
func TestFetchDisabledReason(t *testing.T) {
	// Gate off.
	if r := FetchDisabledReason(&config.ServerConfig{}); r == "" {
		t.Error("expected a reason when update-enabled is off")
	}
	// Gate on, file set, no url.
	if r := FetchDisabledReason(&config.ServerConfig{BaseImagesUpdateEnabled: true, BaseImagesManifest: "/x.toml"}); r == "" {
		t.Error("expected a reason when a file is in use without a url")
	}
	// Fetchable → reason is informational but should still be non-empty.
	if r := FetchDisabledReason(&config.ServerConfig{BaseImagesUpdateEnabled: true}); r == "" {
		t.Error("expected a non-empty reason")
	}
}
