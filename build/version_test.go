package build

import "testing"

func TestIsCompatible(t *testing.T) {
	Version = "0.33.0"
	defer func() { Version = "0.33.0" }()

	tests := []struct {
		name   string
		other  string
		expect bool
	}{
		{"identical", "0.33.0", true},
		{"patch differs", "0.33.7", true},
		{"short other", "0.33", true},
		{"pre-release suffix", "0.33.0-rc1", true},
		{"extra parts", "0.33.1.2", true},
		{"minor differs", "0.34.0", false},
		{"major differs", "1.33.0", false},
		{"older minor", "0.32.4", false},
		{"empty", "", false},
		{"garbage", "unknown", false},
		{"bare number", "0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCompatible(tt.other); got != tt.expect {
				t.Errorf("IsCompatible(%q) = %v, want %v", tt.other, got, tt.expect)
			}
		})
	}
}
