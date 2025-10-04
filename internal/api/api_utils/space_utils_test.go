package api_utils

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestGetSpaceDetailsValidation(t *testing.T) {
	user := &model.User{
		Id:    "user-123",
		Roles: []string{},
	}

	tests := []struct {
		name        string
		spaceId     string
		expectError bool
	}{
		{
			name:        "empty space ID",
			spaceId:     "",
			expectError: true,
		},
		{
			name:        "invalid UUID",
			spaceId:     "not-a-uuid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetSpaceDetails(tt.spaceId, user)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
