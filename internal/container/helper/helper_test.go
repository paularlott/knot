package helper

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestNewContainerHelper(t *testing.T) {
	helper := NewContainerHelper()
	if helper == nil {
		t.Fatal("NewContainerHelper returned nil")
	}
}

func TestCreateClient(t *testing.T) {
	helper := NewContainerHelper()

	tests := []struct {
		name        string
		platform    string
		expectError bool
	}{
		{
			name:        "unsupported platform",
			platform:    "unsupported",
			expectError: true,
		},
		{
			name:        "manual platform",
			platform:    model.PlatformManual,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := helper.createClient(tt.platform)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				if client != nil {
					t.Error("Expected nil client on error")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if client == nil {
					t.Error("Expected non-nil client")
				}
			}
		})
	}
}
