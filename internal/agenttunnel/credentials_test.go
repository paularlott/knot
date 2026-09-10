package agenttunnel

import "testing"

func TestCredentials(t *testing.T) {
	cases := []struct {
		name            string
		server          string
		token           string
		skipVerify      bool
		agentServer     string
		agentToken      string
		agentSkipVerify bool
		wantServer      string
		wantToken       string
		wantSkipVerify  bool
		wantErr         bool
	}{
		{
			name:            "no target uses agent server",
			agentServer:     "https://owning.example.com",
			agentToken:      "tok-agent",
			agentSkipVerify: true,
			wantServer:      "https://owning.example.com",
			wantToken:       "tok-agent",
			wantSkipVerify:  true,
		},
		{
			name:            "request server and token win",
			server:          "https://other.example.com",
			token:           "tok-other",
			skipVerify:      true,
			agentServer:     "https://owning.example.com",
			agentToken:      "tok-agent",
			agentSkipVerify: false,
			wantServer:      "https://other.example.com",
			wantToken:       "tok-other",
			wantSkipVerify:  true,
		},
		{
			name:           "request without skip verify verifies",
			server:         "https://other.example.com",
			token:          "tok-other",
			wantServer:     "https://other.example.com",
			wantToken:      "tok-other",
			wantSkipVerify: false,
		},
		{
			name:        "server without token is an error",
			server:      "https://other.example.com",
			agentServer: "https://owning.example.com",
			agentToken:  "tok-agent",
			wantErr:     true,
		},
		{
			name:        "token without server is an error",
			token:       "tok-other",
			agentServer: "https://owning.example.com",
			agentToken:  "tok-agent",
			wantErr:     true,
		},
		{
			name:    "no target and no agent credentials is an error",
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server, token, skipVerify, err := Credentials(c.server, c.token, c.skipVerify, c.agentServer, c.agentToken, c.agentSkipVerify)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got server=%q token=%q", server, token)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if server != c.wantServer || token != c.wantToken || skipVerify != c.wantSkipVerify {
				t.Fatalf("got (%q, %q, %v), want (%q, %q, %v)", server, token, skipVerify, c.wantServer, c.wantToken, c.wantSkipVerify)
			}
		})
	}
}
