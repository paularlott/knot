package kvm

import (
	"testing"
)

func TestQuoteEnvEntry(t *testing.T) {
	cases := map[string]string{
		"TEST=Testing env 1":      "TEST='Testing env 1'",
		"KNOT_SERVER=https://x/":  "KNOT_SERVER='https://x/'",
		"EMPTY=":                  "EMPTY=''",
		"Q=it's here":             "Q='it'\\''s here'",
		"PORTS=web=8080,api=9090": "PORTS='web=8080,api=9090'",
	}
	for in, want := range cases {
		if got := quoteEnvEntry(in); got != want {
			t.Errorf("quoteEnvEntry(%q) = %q, want %q", in, got, want)
		}
	}
	// no-key entries pass through untouched
	if got := quoteEnvEntry("MALFORMED"); got != "MALFORMED" {
		t.Errorf("malformed entry changed: %q", got)
	}
}
