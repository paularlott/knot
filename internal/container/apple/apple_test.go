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
