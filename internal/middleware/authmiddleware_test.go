package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

// TestMain installs a non-nil ServerConfig so package-level helpers that read
// it (e.g. DeleteSessionCookie via GetServerConfig) don't segfault when an
// unauthenticated request triggers returnUnauthorized.
func TestMain(m *testing.M) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{})
	defer config.SetServerConfig(prev)
	m.Run()
}

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
		name            string
		userRoles       []string
		permission      uint16
		expectForbidden bool
	}{
		{
			name:            "user has permission",
			userRoles:       []string{"role1"},
			permission:      model.PermissionManageTemplates,
			expectForbidden: false,
		},
		{
			name:            "user lacks permission",
			userRoles:       []string{"role1"},
			permission:      model.PermissionManageUsers,
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

// A busy client does not write its token to the database, and gossip it, on
// every request: only when its expiry has fallen an hour behind.
func TestTokenNeedsExtending(t *testing.T) {
	now := time.Now()
	for name, tc := range map[string]struct {
		expires time.Time
		want    bool
	}{
		"just extended":       {now.Add(model.MaxTokenAge), false},
		"a minute ago":        {now.Add(model.MaxTokenAge - time.Minute), false},
		"just under the hour": {now.Add(model.MaxTokenAge - 59*time.Minute), false},
		"an hour and a bit":   {now.Add(model.MaxTokenAge - 61*time.Minute), true},
		"a week left":         {now.Add(7 * 24 * time.Hour), true},
		"about to expire":     {now.Add(time.Minute), true},
		"already expired":     {now.Add(-time.Hour), true},
	} {
		if got := tokenNeedsExtending(&model.Token{ExpiresAfter: tc.expires}, now); got != tc.want {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
}

func TestTokenNotFound(t *testing.T) {
	for msg, want := range map[string]bool{
		"token not found":                  true,
		"Key not found":                    true,
		"sql: no rows in result set":       true,
		"redis: nil":                       true,
		"Error 1040: Too many connections": false,
		"dial tcp 127.0.0.1:3306: connect: can't assign requested address": false,
		"context deadline exceeded":                                        false,
	} {
		if got := tokenNotFound(errors.New(msg)); got != want {
			t.Errorf("%q: %v, want %v", msg, got, want)
		}
	}
}

// Backing up and restoring both need the Backup Server permission, and a
// server with no users has no open window: there is no one to hold it.
func TestBackupPermissions(t *testing.T) {
	prev := HasUsers
	defer func() { HasUsers = prev }()

	call := func(wrap func(http.HandlerFunc) http.HandlerFunc, user *model.User) int {
		h := wrap(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
		r := httptest.NewRequest("POST", "/", nil)
		if user != nil {
			r = r.WithContext(context.WithValue(r.Context(), "user", user))
		}
		w := httptest.NewRecorder()
		h(w, r)
		return w.Code
	}

	for name, wrap := range map[string]func(http.HandlerFunc) http.HandlerFunc{"backup": ApiPermissionBackup, "restore": ApiPermissionRestore} {
		HasUsers = false
		if code := call(wrap, nil); code != http.StatusForbidden {
			t.Errorf("%s on a server with no users: %d, want 403", name, code)
		}
		HasUsers = true
		if code := call(wrap, nil); code != http.StatusForbidden {
			t.Errorf("%s without a user: %d, want 403", name, code)
		}
		if code := call(wrap, &model.User{}); code != http.StatusForbidden {
			t.Errorf("%s by a user without the permission: %d, want 403", name, code)
		}
	}
}
