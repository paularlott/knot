package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paularlott/cli"
	cli_toml "github.com/paularlott/cli/toml"
)

func TestNewServerAddr(t *testing.T) {
	cases := []struct {
		name     string
		server   string
		wantHTTP string
		wantWS   string
	}{
		{"bare host gets https", "knot.example.com", "https://knot.example.com", "wss://knot.example.com"},
		{"https preserved", "https://knot.example.com", "https://knot.example.com", "wss://knot.example.com"},
		{"http preserved with ws", "http://knot.example.com", "http://knot.example.com", "ws://knot.example.com"},
		{"trailing slash trimmed", "https://knot.example.com/", "https://knot.example.com", "wss://knot.example.com"},
		{"host with port", "knot.example.com:3000", "https://knot.example.com:3000", "wss://knot.example.com:3000"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			addr := NewServerAddr(c.server, "tok")
			if addr.HttpServer != c.wantHTTP {
				t.Fatalf("HttpServer = %q, want %q", addr.HttpServer, c.wantHTTP)
			}
			if addr.WsServer != c.wantWS {
				t.Fatalf("WsServer = %q, want %q", addr.WsServer, c.wantWS)
			}
			if addr.ApiToken != "tok" {
				t.Fatalf("ApiToken = %q, want %q", addr.ApiToken, "tok")
			}
		})
	}
}

// newCmdWithConfig builds a cli.Command whose config file is a temp knot.toml
// containing the given content.
func newCmdWithConfig(t *testing.T, content string) *cli.Command {
	t.Helper()

	path := filepath.Join(t.TempDir(), "knot.toml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	return &cli.Command{ConfigFile: cli_toml.NewConfigFile(&path, nil)}
}

func TestLookupServerAddr(t *testing.T) {
	cmd := newCmdWithConfig(t, `
[client.connection.default]
server = "https://a.example.com"
token = "tok-a"

[client.connection.work]
server = "b.example.com/"
token = "tok-b"

[client.connection.half]
server = "https://c.example.com"
`)

	t.Run("configured alias resolves", func(t *testing.T) {
		addr, ok := LookupServerAddr("default", cmd)
		if !ok {
			t.Fatal("expected default alias to resolve")
		}
		if addr.HttpServer != "https://a.example.com" || addr.WsServer != "wss://a.example.com" || addr.ApiToken != "tok-a" {
			t.Fatalf("unexpected addr: %+v", addr)
		}
	})

	t.Run("server url normalised", func(t *testing.T) {
		addr, ok := LookupServerAddr("work", cmd)
		if !ok {
			t.Fatal("expected work alias to resolve")
		}
		if addr.HttpServer != "https://b.example.com" || addr.WsServer != "wss://b.example.com" || addr.ApiToken != "tok-b" {
			t.Fatalf("unexpected addr: %+v", addr)
		}
	})

	t.Run("alias without token does not resolve", func(t *testing.T) {
		if _, ok := LookupServerAddr("half", cmd); ok {
			t.Fatal("expected alias missing a token to not resolve")
		}
	})

	t.Run("unknown alias does not resolve", func(t *testing.T) {
		if _, ok := LookupServerAddr("nope", cmd); ok {
			t.Fatal("expected unknown alias to not resolve")
		}
	})

	t.Run("invalid alias does not resolve", func(t *testing.T) {
		for _, alias := range []string{"", "1abc", "has space", "-lead", "Upper", "a-b-c-d-e-f-g-h-i-j-k-l-m-n-o-p"} {
			if _, ok := LookupServerAddr(alias, cmd); ok {
				t.Fatalf("expected alias %q to be rejected", alias)
			}
		}
	})

	t.Run("no config file does not resolve", func(t *testing.T) {
		if _, ok := LookupServerAddr("default", &cli.Command{}); ok {
			t.Fatal("expected nil config file to not resolve")
		}
	})
}
