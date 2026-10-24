package util

import "testing"

func TestCanOpenBrowser(t *testing.T) {
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want bool
	}{
		{"macOS desktop", "darwin", nil, true},
		{"windows desktop", "windows", nil, true},
		{"linux X11", "linux", map[string]string{"DISPLAY": ":0"}, true},
		{"linux wayland", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, true},
		{"linux console", "linux", nil, false},
		{"ssh into macOS", "darwin", map[string]string{"SSH_CONNECTION": "10.0.0.1 50000 10.0.0.2 22"}, false},
		{"ssh with X forwarding", "linux", map[string]string{"SSH_TTY": "/dev/pts/0", "DISPLAY": "localhost:10.0"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canOpenBrowser(tt.goos, func(k string) string { return tt.env[k] })
			if got != tt.want {
				t.Errorf("canOpenBrowser = %v, want %v", got, tt.want)
			}
		})
	}
}
