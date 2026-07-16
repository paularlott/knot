package config

import "testing"

func TestSessionCookieDomain(t *testing.T) {
	cases := []struct {
		name           string
		wildcardDomain string
		serverURL      string
		want           string
	}{
		{"no wildcard", "", "https://knot.example.com", ""},
		{"wildcard parent of server", "*.example.com", "https://knot.example.com", "example.com"},
		{"wildcard nested parent of server", "*.knot.example.com", "https://knot.example.com", "knot.example.com"},
		{"server equals wildcard parent", "*.example.com", "https://example.com", "example.com"},
		{"server not under wildcard (reject)", "*.other.com", "https://knot.example.com", ""},
		{"star without leading dot", "example.com", "https://knot.example.com", "example.com"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &ServerConfig{WildcardDomain: c.wildcardDomain, URL: c.serverURL}
			got := cfg.SessionCookieDomain()
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}
