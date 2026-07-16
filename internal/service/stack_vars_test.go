package service

import (
	"testing"

	"github.com/paularlott/knot/internal/util/validate"
)

func TestStackKeyAlias(t *testing.T) {
	cases := []struct{ in, want string }{
		{"space-1", "space_1"},
		{"db", "db"},   // no hyphen -> unchanged
		{"a-b-c", "a_b_c"},
		{"my-db", "my_db"},
	}
	for _, c := range cases {
		if got := stackKeyAlias(c.in); got != c.want {
			t.Fatalf("stackKeyAlias(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestStackAliasInvariant documents the load-bearing assumption that makes the
// dotted "-" -> "_" alias collision-free: space names (and thus stack keys) can
// never contain "_". If this ever changes, the aliasing in BuildStackVariableData
// could let two distinct siblings shadow one another, so this test must keep
// passing (or the aliasing strategy must be revisited).
func TestStackAliasInvariant(t *testing.T) {
	if validate.Name("has_underscore") {
		t.Fatal("space names must not allow '_' (URL safety); the .stack dotted alias depends on this")
	}
	if !validate.Name("has-hyphen") {
		t.Fatal("expected hyphenated names to be valid")
	}
}
