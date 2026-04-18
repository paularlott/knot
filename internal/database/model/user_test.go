package model

import (
	"testing"
)

func TestNewUser(t *testing.T) {
	roles := []string{"role1", "role2"}
	groups := []string{"group1"}

	user := NewUser("testuser", "test@example.com", "password123", roles, groups, "ssh-key", "/bin/bash", "UTC", 5, "githubuser", 100, 200, 3)

	if user.Id == "" {
		t.Error("User ID should not be empty")
	}
	if user.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", user.Username)
	}
	if user.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s'", user.Email)
	}
	if !user.Active {
		t.Error("New user should be active")
	}
	if user.SSHPublicKey != "ssh-key" {
		t.Errorf("Expected SSH key 'ssh-key', got '%s'", user.SSHPublicKey)
	}
	if user.PreferredShell != "/bin/bash" {
		t.Errorf("Expected shell '/bin/bash', got '%s'", user.PreferredShell)
	}
	if len(user.Roles) != 2 {
		t.Errorf("Expected 2 roles, got %d", len(user.Roles))
	}
	if len(user.Groups) != 1 {
		t.Errorf("Expected 1 group, got %d", len(user.Groups))
	}
	if user.MaxSpaces != 5 {
		t.Errorf("Expected max spaces 5, got %d", user.MaxSpaces)
	}
	if user.ComputeUnits != 100 {
		t.Errorf("Expected compute units 100, got %d", user.ComputeUnits)
	}
	if user.StorageUnits != 200 {
		t.Errorf("Expected storage units 200, got %d", user.StorageUnits)
	}
	if user.MaxTunnels != 3 {
		t.Errorf("Expected max tunnels 3, got %d", user.MaxTunnels)
	}
	if user.ServicePassword == "" {
		t.Error("Service password should be generated")
	}
	if len(user.ServicePassword) != 16 {
		t.Errorf("Expected service password length 16, got %d", len(user.ServicePassword))
	}
}

func TestSetPassword(t *testing.T) {
	user := &User{}
	err := user.SetPassword("testpassword")
	if err != nil {
		t.Fatalf("SetPassword failed: %v", err)
	}
	if user.Password == "" {
		t.Error("Password should be set")
	}
	if user.Password == "testpassword" {
		t.Error("Password should be hashed, not plain text")
	}
}

func TestCheckPassword(t *testing.T) {
	user := &User{}
	password := "testpassword"
	user.SetPassword(password)

	if !user.CheckPassword(password) {
		t.Error("CheckPassword should return true for correct password")
	}
	if user.CheckPassword("wrongpassword") {
		t.Error("CheckPassword should return false for incorrect password")
	}
}

func TestHasPermission(t *testing.T) {
	// Setup role cache for testing
	roleCache = map[string]*Role{
		"role1": {
			Id:          "role1",
			Permissions: []uint16{PermissionManageSpaces, PermissionUseSpaces},
		},
		"role2": {
			Id:          "role2",
			Permissions: []uint16{PermissionManageUsers},
		},
	}

	tests := []struct {
		name       string
		userRoles  []string
		permission uint16
		expected   bool
	}{
		{
			name:       "has permission from first role",
			userRoles:  []string{"role1"},
			permission: PermissionManageSpaces,
			expected:   true,
		},
		{
			name:       "has permission from second role",
			userRoles:  []string{"role1", "role2"},
			permission: PermissionManageUsers,
			expected:   true,
		},
		{
			name:       "does not have permission",
			userRoles:  []string{"role1"},
			permission: PermissionManageUsers,
			expected:   false,
		},
		{
			name:       "no roles",
			userRoles:  []string{},
			permission: PermissionManageSpaces,
			expected:   false,
		},
		{
			name:       "role not in cache",
			userRoles:  []string{"role3"},
			permission: PermissionManageSpaces,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{Roles: tt.userRoles}
			result := user.HasPermission(tt.permission)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHasAnyGroup(t *testing.T) {
	tests := []struct {
		name       string
		userGroups []string
		testGroups []string
		expected   bool
	}{
		{
			name:       "has matching group",
			userGroups: []string{"group1", "group2"},
			testGroups: []string{"group2", "group3"},
			expected:   true,
		},
		{
			name:       "no matching group",
			userGroups: []string{"group1", "group2"},
			testGroups: []string{"group3", "group4"},
			expected:   false,
		},
		{
			name:       "user has no groups",
			userGroups: []string{},
			testGroups: []string{"group1"},
			expected:   false,
		},
		{
			name:       "empty test groups",
			userGroups: []string{"group1"},
			testGroups: []string{},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{Groups: tt.userGroups}
			result := user.HasAnyGroup(&tt.testGroups)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsAdmin(t *testing.T) {
	tests := []struct {
		name     string
		roles    []string
		expected bool
	}{
		{
			name:     "is admin",
			roles:    []string{RoleAdminUUID, "other-role"},
			expected: true,
		},
		{
			name:     "not admin",
			roles:    []string{"role1", "role2"},
			expected: false,
		},
		{
			name:     "no roles",
			roles:    []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{Roles: tt.roles}
			result := user.IsAdmin()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSetOAuthTokensPreservesExistingRefreshToken(t *testing.T) {
	user := &User{
		ExternalAuthProviders: map[string]ExternalProvider{
			"google": {ProviderUID: "123", Username: "alice"},
		},
	}
	key := "0123456789abcdef0123456789abcdef"

	user.SetOAuthTokens("google", "access-1", "refresh-1", key)
	if got := user.GetOAuthToken("google", key); got != "access-1" {
		t.Fatalf("GetOAuthToken() = %q, want access-1", got)
	}
	if got := user.GetOAuthRefreshToken("google", key); got != "refresh-1" {
		t.Fatalf("GetOAuthRefreshToken() = %q, want refresh-1", got)
	}

	user.SetOAuthTokens("google", "access-2", "", key)
	if got := user.GetOAuthToken("google", key); got != "access-2" {
		t.Fatalf("GetOAuthToken() after update = %q, want access-2", got)
	}
	if got := user.GetOAuthRefreshToken("google", key); got != "refresh-1" {
		t.Fatalf("GetOAuthRefreshToken() should be preserved, got %q", got)
	}
}

func TestClearOAuthTokens(t *testing.T) {
	user := &User{
		ExternalAuthProviders: map[string]ExternalProvider{
			"github": {ProviderUID: "123", Username: "alice"},
		},
	}
	key := "0123456789abcdef0123456789abcdef"

	user.SetOAuthTokens("github", "access", "refresh", key)
	user.ClearOAuthTokens("github")

	if got := user.GetOAuthToken("github", key); got != "" {
		t.Fatalf("GetOAuthToken() = %q, want empty", got)
	}
	if got := user.GetOAuthRefreshToken("github", key); got != "" {
		t.Fatalf("GetOAuthRefreshToken() = %q, want empty", got)
	}
}
