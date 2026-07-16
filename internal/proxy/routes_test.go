package proxy

import "testing"

func TestIsVNCSubdomain(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"alice--myapp--vnc.example.com", true},
		{"alice--myapp--vnc.knot.example.com", true},
		{"alice--myapp--vnc.example.com:8443", true},
		{"alice--myapp--8080.example.com", false},
		{"alice--myapp.example.com", false},
		{"knot.example.com", false},
		{"", false},
	}

	for _, c := range cases {
		t.Run(c.host, func(t *testing.T) {
			if got := isVNCSubdomain(c.host); got != c.want {
				t.Fatalf("isVNCSubdomain(%q) = %v, want %v", c.host, got, c.want)
			}
		})
	}
}
