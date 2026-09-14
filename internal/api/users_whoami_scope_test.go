package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

// Scoped API tokens may read their owner's identity at /api/users/whoami
// (the tunnel CLI needs the username to build tunnel URLs) but must not
// receive credential fields.
func TestWhoAmIWithholdsCredentialsFromScopedTokens(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	user := &model.User{
		Id:              "user-1",
		Username:        "paul",
		Email:           "paul@example.com",
		ServicePassword: "service-pass",
		SSHPublicKey:    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExAbXBsZQ==",
	}

	cases := []struct {
		name                string
		token               *model.Token
		wantServicePassword string
		wantSSHPublicKey    string
	}{
		{
			name:                "unscoped token receives credentials",
			token:               &model.Token{Id: "tok-full"},
			wantServicePassword: "service-pass",
			wantSSHPublicKey:    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExAbXBsZQ==",
		},
		{
			name:                "tunnels-scoped token receives identity only",
			token:               &model.Token{Id: "tok-tunnels", Scopes: []string{model.ScopeTunnels}},
			wantServicePassword: "",
			wantSSHPublicKey:    "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/users/whoami", nil)
			ctx := context.WithValue(req.Context(), "user", user)
			ctx = context.WithValue(ctx, "access_token", tc.token)
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()

			HandleWhoAmI(rr, req)

			if rr.Code != 200 {
				t.Fatalf("status = %d, want 200", rr.Code)
			}

			var resp struct {
				Username        string `json:"username"`
				ServicePassword string `json:"service_password"`
				SSHPublicKey    string `json:"ssh_public_key"`
				SSHPrivateKey   string `json:"ssh_private_key"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}

			if resp.Username != "paul" {
				t.Errorf("username = %q, want %q (identity must stay readable)", resp.Username, "paul")
			}
			if resp.ServicePassword != tc.wantServicePassword {
				t.Errorf("service_password = %q, want %q", resp.ServicePassword, tc.wantServicePassword)
			}
			if resp.SSHPublicKey != tc.wantSSHPublicKey {
				t.Errorf("ssh_public_key = %q, want %q", resp.SSHPublicKey, tc.wantSSHPublicKey)
			}
			if resp.SSHPrivateKey != "" {
				t.Errorf("ssh_private_key = %q, want empty", resp.SSHPrivateKey)
			}
		})
	}
}
