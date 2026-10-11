package specwizard

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
)

// TestFetchDecision locks the fetch rule. Notation: M = manifest file set,
// A = update-enabled on, U = update-url set. Startup and manual use the same
// decision: fetch iff A && U (there is no default URL, so the gate alone is
// never enough); the URL is always the configured one.
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
		name    string
		m, a, u bool
		wantOK  bool
	}{
		// Gate off → never fetch, regardless of M/U.
		{"A=F !M !U", false, false, false, false},
		{"A=F !M  U", false, false, true, false},
		{"A=F  M !U", true, false, false, false},
		{"A=F  M  U", true, false, true, false},

		// Gate on but no URL → never fetch: knot looks nowhere by default.
		{"A=T !M !U", false, true, false, false},
		{"A=T  M !U", true, true, false, false},

		// Gate on with a URL → always fetch, file or not (file is baseline).
		{"A=T !M  U", false, true, true, true},
		{"A=T  M  U", true, true, true, true},
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
		if url != "https://example.com/given.toml" {
			t.Errorf("%s: got url %q, want the configured url", c.name, url)
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
	// Gate on but no URL configured.
	if r := FetchDisabledReason(&config.ServerConfig{BaseImagesUpdateEnabled: true}); r == "" {
		t.Error("expected a reason when no update URL is configured")
	}
	// Fetchable → empty (no reason to report).
	if r := FetchDisabledReason(&config.ServerConfig{BaseImagesUpdateEnabled: true, BaseImagesUpdateURL: "https://example.com/x.toml"}); r != "" {
		t.Errorf("expected no reason for a fetchable config, got %q", r)
	}
}
