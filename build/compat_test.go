package build

import "testing"

func TestSameMajorMinor(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"0.41.0", "0.41.7", true},
		{"v0.41.0", "0.41.0", true},
		{"0.41.0", "0.40.9", false},
		{"1.41.0", "0.41.0", false},
		{"0.41", "0.41.3", true},
		{"", "0.41.0", false},
		{"dev", "dev", false},
	} {
		if got := SameMajorMinor(tc.a, tc.b); got != tc.want {
			t.Errorf("SameMajorMinor(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
