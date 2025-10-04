package util

import (
	"testing"
)

func TestFixListenAddress(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "port only",
			input:    "8080",
			expected: ":8080",
		},
		{
			name:     "full address",
			input:    "0.0.0.0:8080",
			expected: "0.0.0.0:8080",
		},
		{
			name:     "localhost with port",
			input:    "localhost:8080",
			expected: "localhost:8080",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "colon with port",
			input:    ":9000",
			expected: ":9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FixListenAddress(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGetLocalIP(t *testing.T) {
	ip, err := GetLocalIP()
	if err != nil {
		t.Skipf("Skipping test, no network interfaces available: %v", err)
	}

	if ip == "" {
		t.Error("Expected non-empty IP address")
	}

	if ip == "127.0.0.1" {
		t.Error("Should not return loopback address")
	}
}
