package apple

import (
	"testing"

	"github.com/paularlott/knot/internal/container"
)

func TestAppleClientImplementsInterface(t *testing.T) {
	var _ container.ContainerManager = (*AppleClient)(nil)
}

func TestNewClient(t *testing.T) {
	client := NewClient()
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.DriverName != "apple" {
		t.Errorf("Expected DriverName to be 'apple', got '%s'", client.DriverName)
	}
}

func TestNormalizeContainerReference(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain id",
			in:   "abc123\n",
			want: "abc123",
		},
		{
			name: "apple cli progress output",
			in:   "[0/6] [0s]\n[1/6] Fetching image [0s]\n[6/6] Starting container [0s]\npaul-mtest\n",
			want: "paul-mtest",
		},
		{
			name: "carriage returns and whitespace",
			in:   "\r\n  line-one  \r\n  final-name  \r\n",
			want: "final-name",
		},
		{
			name: "empty output",
			in:   " \n\t\r\n",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeContainerReference(tt.in)
			if got != tt.want {
				t.Fatalf("normalizeContainerReference(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsIgnorableAppleCleanupOutput(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool
	}{
		{name: "not found", out: "volume not found", want: true},
		{name: "case insensitive no such", out: "No Such volume", want: true},
		{name: "does not exist", out: "container does not exist", want: true},
		{name: "unable to find", out: "unable to find volume", want: true},
		{name: "real failure", out: "volume is in use", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isIgnorableAppleCleanupOutput(tt.out)
			if got != tt.want {
				t.Fatalf("isIgnorableAppleCleanupOutput(%q) = %v, want %v", tt.out, got, tt.want)
			}
		})
	}
}
