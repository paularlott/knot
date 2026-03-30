package rest

import (
	"testing"
)

func TestParseDockerHost(t *testing.T) {
	tests := []struct {
		input          string
		wantSocketPath string
		wantIsTCP      bool
		wantTCPBase    string
	}{
		{"", "/var/run/docker.sock", false, ""},
		{"unix:///var/run/docker.sock", "/var/run/docker.sock", false, ""},
		{"unix:///run/podman/podman.sock", "/run/podman/podman.sock", false, ""},
		{"tcp://localhost:2375", "", true, "http://localhost:2375"},
		{"http://localhost:2375", "", true, "http://localhost:2375"},
		{"https://localhost:2376", "", true, "https://localhost:2376"},
	}

	for _, tt := range tests {
		socketPath, isTCP, tcpBase := parseDockerHost(tt.input)
		if socketPath != tt.wantSocketPath {
			t.Errorf("parseDockerHost(%q) socketPath = %q, want %q", tt.input, socketPath, tt.wantSocketPath)
		}
		if isTCP != tt.wantIsTCP {
			t.Errorf("parseDockerHost(%q) isTCP = %v, want %v", tt.input, isTCP, tt.wantIsTCP)
		}
		if tcpBase != tt.wantTCPBase {
			t.Errorf("parseDockerHost(%q) tcpBase = %q, want %q", tt.input, tcpBase, tt.wantTCPBase)
		}
	}
}

func TestNewUnixSocketClient(t *testing.T) {
	tests := []struct {
		host        string
		wantBaseURL string
	}{
		{"", "http://localhost"},
		{"unix:///var/run/docker.sock", "http://localhost"},
		{"tcp://localhost:2375", "http://localhost:2375"},
		{"http://localhost:2375", "http://localhost:2375"},
	}

	for _, tt := range tests {
		c, err := NewUnixSocketClient(tt.host)
		if err != nil {
			t.Errorf("NewUnixSocketClient(%q) unexpected error: %v", tt.host, err)
			continue
		}
		if c.GetBaseURL() != tt.wantBaseURL {
			t.Errorf("NewUnixSocketClient(%q) baseURL = %q, want %q", tt.host, c.GetBaseURL(), tt.wantBaseURL)
		}
		if c.HTTPClient == nil {
			t.Errorf("NewUnixSocketClient(%q) HTTPClient is nil", tt.host)
		}
		if c.HTTPClient.Transport == nil {
			t.Errorf("NewUnixSocketClient(%q) Transport is nil", tt.host)
		}
	}
}
