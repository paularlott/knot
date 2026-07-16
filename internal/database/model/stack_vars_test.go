package model

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
)

func TestResolveVariablesStack(t *testing.T) {
	config.SetServerConfig(&config.ServerConfig{})

	// Mock the stack resolver (normally injected by the service layer) to expose
	// a single sibling "db" with a custom field.
	prev := stackResolver
	stackResolver = func(space *Space, variables map[string]interface{}) map[string]interface{} {
		if space == nil || space.Stack == "" {
			return nil
		}
		return map[string]interface{}{
			"db": map[string]interface{}{
				"space": map[string]interface{}{
					"id":   "db-space-id",
					"name": "myapp-db",
				},
				"custom": map[string]interface{}{
					"password": "s3cr3t",
				},
			},
		}
	}
	defer func() { stackResolver = prev }()

	space := &Space{Id: "web-space-id", Name: "myapp-web", Stack: "myapp", StackPrefix: "myapp"}

	t.Run("resolves sibling custom variable", func(t *testing.T) {
		out, err := ResolveVariables(`${{ .stack.db.custom.password }}`, &Template{}, space, nil, nil)
		if err != nil {
			t.Fatalf("ResolveVariables returned error: %v", err)
		}
		if out != "s3cr3t" {
			t.Fatalf("got %q, want %q", out, "s3cr3t")
		}
	})

	t.Run("resolves sibling system variable", func(t *testing.T) {
		out, err := ResolveVariables(`DB_ID=${{ .stack.db.space.id }}`, &Template{}, space, nil, nil)
		if err != nil {
			t.Fatalf("ResolveVariables returned error: %v", err)
		}
		if out != "DB_ID=db-space-id" {
			t.Fatalf("got %q, want %q", out, "DB_ID=db-space-id")
		}
	})

	t.Run("stack absent when space has no stack", func(t *testing.T) {
		standalone := &Space{Id: "s", Name: "solo"}
		out, err := ResolveVariables(`[${{ .stack.db.custom.password }}]`, &Template{}, standalone, nil, nil)
		if err != nil {
			t.Fatalf("ResolveVariables returned error: %v", err)
		}
		// No stack -> the reference is a missing key, which Go's text/template
		// renders as "<no value>" (same as any other missing variable).
		if out != "[<no value>]" {
			t.Fatalf("got %q, want %q", out, "[<no value>]")
		}
	})

	// A hyphenated sibling key is exposed under both its literal form (for
	// index) and a dotted-safe "_" alias by the service-layer resolver.
	t.Run("hyphenated key resolves via dotted alias and index", func(t *testing.T) {
		prev2 := stackResolver
		sibling := map[string]interface{}{
			"space":  map[string]interface{}{"id": "space-1-id"},
			"custom": map[string]interface{}{"password": "hyphen-secret"},
		}
		stackResolver = func(*Space, map[string]interface{}) map[string]interface{} {
			return map[string]interface{}{
				"space-1": sibling,
				"space_1": sibling, // dotted-safe alias
			}
		}
		defer func() { stackResolver = prev2 }()

		out, err := ResolveVariables(`DOTTED=${{ .stack.space_1.custom.password }} INDEX=${{ (index .stack "space-1").space.id }}`, &Template{}, space, nil, nil)
		if err != nil {
			t.Fatalf("ResolveVariables returned error: %v", err)
		}
		if want := "DOTTED=hyphen-secret INDEX=space-1-id"; out != want {
			t.Fatalf("got %q, want %q", out, want)
		}
	})
}
