package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestGetBearerToken(t *testing.T) {
	tests := []struct {
		name        string
		authHeader  string
		expectEmpty bool
	}{
		{
			name:        "valid bearer token",
			authHeader:  "Bearer test-token-123",
			expectEmpty: false,
		},
		{
			name:        "no bearer prefix",
			authHeader:  "test-token-123",
			expectEmpty: true,
		},
		{
			name:        "empty header",
			authHeader:  "",
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			token := GetBearerToken(w, req)

			if tt.expectEmpty && token != "" {
				t.Errorf("Expected empty token, got %q", token)
			}
			if !tt.expectEmpty && token == "" {
				t.Error("Expected non-empty token")
			}
		})
	}
}

func TestCheckPermissionLogic(t *testing.T) {
	// Setup role cache
	model.SetRoleCache([]*model.Role{
		{
			Id:          "role1",
			Permissions: []uint16{model.PermissionManageTemplates},
		},
	})

	tests := []struct {
		name           string
		userRoles      []string
		permission     uint16
		expectForbidden bool
	}{
		{
			name:           "user has permission",
			userRoles:      []string{"role1"},
			permission:     model.PermissionManageTemplates,
			expectForbidden: false,
		},
		{
			name:           "user lacks permission",
			userRoles:      []string{"role1"},
			permission:     model.PermissionManageUsers,
			expectForbidden: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &model.User{
				Id:     "user-123",
				Roles:  tt.userRoles,
				Active: true,
			}

			hasPermission := user.HasPermission(tt.permission)

			if tt.expectForbidden && hasPermission {
				t.Error("Expected user to lack permission")
			}
			if !tt.expectForbidden && !hasPermission {
				t.Error("Expected user to have permission")
			}
		})
	}
}
