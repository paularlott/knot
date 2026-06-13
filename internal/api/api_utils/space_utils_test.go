package api_utils

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
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

func TestGetNodeHostnameWithoutTransport(t *testing.T) {
	service.SetTransport(nil)

	tests := []struct {
		name             string
		nodeId           string
		isRemote         bool
		fallbackHostname string
		want             string
	}{
		{
			name:             "local node falls back to server hostname",
			nodeId:           "local-node",
			fallbackHostname: "local-host",
			want:             "local-host",
		},
		{
			name:     "remote node reports offline",
			nodeId:   "remote-node",
			isRemote: true,
			want:     "Offline Remote Node",
		},
		{
			name: "empty node id stays empty",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getNodeHostname(tt.nodeId, tt.isRemote, tt.fallbackHostname)
			if got != tt.want {
				t.Fatalf("getNodeHostname() = %q, want %q", got, tt.want)
			}
		})
	}
}
