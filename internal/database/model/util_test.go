package model

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
)

func TestResolveVariablesStackFields(t *testing.T) {
	// ResolveVariables reads the global server config; ensure it is non-nil.
	config.SetServerConfig(&config.ServerConfig{})

	t.Run("exposes stack and stack_prefix from the space", func(t *testing.T) {
		space := &Space{
			Id:          "space-1",
			Name:        "myapp-web",
			Stack:       "myapp",
			StackPrefix: "myapp",
		}

		out, err := ResolveVariables(`${{ .space.stack }}|${{ .space.stack_prefix }}`, nil, space, nil, nil)
		if err != nil {
			t.Fatalf("ResolveVariables returned error: %v", err)
		}
		if out != "myapp|myapp" {
			t.Fatalf("got %q, want %q", out, "myapp|myapp")
		}
	})

	t.Run("stack_prefix lets a template reference a sibling container", func(t *testing.T) {
		space := &Space{
			Name:        "prod-web",
			Stack:       "prod",
			StackPrefix: "prod",
		}

		out, err := ResolveVariables(`DATABASE_URL=postgres://${{ .space.stack_prefix }}-db:5432/app`, nil, space, nil, nil)
		if err != nil {
			t.Fatalf("ResolveVariables returned error: %v", err)
		}
		want := "DATABASE_URL=postgres://prod-db:5432/app"
		if out != want {
			t.Fatalf("got %q, want %q", out, want)
		}
	})

	t.Run("defaults to empty when no space", func(t *testing.T) {
		out, err := ResolveVariables(`[${{ .space.stack }}|${{ .space.stack_prefix }}]`, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("ResolveVariables returned error: %v", err)
		}
		if out != "[|]" {
			t.Fatalf("got %q, want %q", out, "[|]")
		}
	})

	t.Run("standalone space has empty stack_prefix", func(t *testing.T) {
		space := &Space{Name: "lonely", Stack: "sometgroup", StackPrefix: ""}

		out, err := ResolveVariables(`${{ .space.stack_prefix }}`, nil, space, nil, nil)
		if err != nil {
			t.Fatalf("ResolveVariables returned error: %v", err)
		}
		if strings.TrimSpace(out) != "" {
			t.Fatalf("got %q, want empty stack_prefix for a standalone space", out)
		}
	})
}

func TestResolveVariablesServerDomains(t *testing.T) {
	render := func(cfg *config.ServerConfig) string {
		config.SetServerConfig(cfg)
		out, err := ResolveVariables(`${{ .server.wildcard_domain }}|${{ .server.tunnel_domain }}`, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("ResolveVariables returned error: %v", err)
		}
		return out
	}

	t.Run("tunnel domain matches the wildcard domain's normalization", func(t *testing.T) {
		// The running config keeps the tunnel domain as a dot-prefixed suffix.
		if got, want := render(&config.ServerConfig{TunnelDomain: ".tunnel.example.com"}), "|.tunnel.example.com"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		// The raw configured form carries the wildcard's leading * instead;
		// only the star is stripped, like the wildcard domain variable.
		if got, want := render(&config.ServerConfig{TunnelDomain: "*.tunnel.example.com"}), "|.tunnel.example.com"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("wildcard and tunnel domains together", func(t *testing.T) {
		cfg := &config.ServerConfig{
			WildcardDomain: "*.knot.example.com",
			TunnelDomain:   ".tunnel.example.com",
		}
		if got, want := render(cfg), ".knot.example.com|.tunnel.example.com"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("both empty when unconfigured", func(t *testing.T) {
		if got, want := render(&config.ServerConfig{}), "|"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}
