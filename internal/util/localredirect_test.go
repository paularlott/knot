package util

import "testing"

func TestLocalRedirect(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"profile path", "/profile", "/profile"},
		{"path with query", "/spaces?filter=mine", "/spaces?filter=mine"},
		{"relative path gets slash", "spaces", "/spaces"},
		{"logout rejected", "/logout", ""},
		{"absolute https URL loses host", "https://attacker.example/phish", "/phish"},
		{"absolute http URL with query", "http://attacker.example/a?b=c", "/a?b=c"},
		{"protocol-relative", "//attacker.example/phish", "/phish"},
		{"protocol-relative in path", "////attacker.example", ""},
		{"backslash rejected", "/\\attacker.example", ""},
		{"fragment dropped", "/profile#frag", "/profile"},
		{"control characters rejected", "/prof\x00ile", ""},
		{"encoded slashes", "/%2F%2Fattacker.example", ""},
		{"encoded slash after slash", "/%2Fattacker.example", ""},
		{"encoded backslash", "/%5Cattacker.example", ""},
		{"encoded tab", "/%09/attacker.example", ""},
		{"double-encoded tab", "/%2509/attacker.example", ""},
		{"double-encoded slash", "/%252F/attacker.example", ""},
		{"encoded newline", "/%0A/attacker.example", ""},
		{"encoding kept", "/files/a%20b", "/files/a%20b"},
		{"encoded query kept", "/oauth/grant?redirect_uri=https%3A%2F%2Fx", "/oauth/grant?redirect_uri=https%3A%2F%2Fx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LocalRedirect(tt.raw)
			if got != tt.want {
				t.Errorf("LocalRedirect(%q) = %q, want %q", tt.raw, got, tt.want)
			}
			// The callback checks the target again after the cookie
			// round trip; that must not change it.
			if again := LocalRedirect(got); got != "" && again != got {
				t.Errorf("LocalRedirect(%q) = %q, then %q", tt.raw, got, again)
			}
		})
	}
}
