package nfsserver

import (
	"net/netip"
	"testing"
)

func TestParseAllow(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"default loopback", nil, []string{"127.0.0.0/8", "::1/128"}},
		{"repeated flag", []string{"10.0.0.0/8", "192.168.1.5"}, []string{"10.0.0.0/8", "192.168.1.5/32"}},
		{"comma list in one value", []string{"10.0.0.0/8, 192.168.1.0/24,"}, []string{"10.0.0.0/8", "192.168.1.0/24"}},
		{"host bits masked", []string{"10.1.2.3/8"}, []string{"10.0.0.0/8"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAllow(tt.in)
			if err != nil {
				t.Fatalf("parseAllow(%q): %v", tt.in, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseAllow(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i, w := range tt.want {
				if got[i] != netip.MustParsePrefix(w) {
					t.Errorf("parseAllow(%q)[%d] = %v, want %v", tt.in, i, got[i], w)
				}
			}
		})
	}

	if _, err := parseAllow([]string{"10.0.0.0/8,not-an-address"}); err == nil {
		t.Error("parseAllow accepted an invalid entry in a comma list")
	}
}
