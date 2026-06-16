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
