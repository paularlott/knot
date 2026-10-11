package driver_postgres

import "testing"

func TestRebind(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"SELECT 1", "SELECT 1"},
		{"a = ? AND b = ?", "a = $1 AND b = $2"},
		{"name = '?' AND id = ?", "name = '?' AND id = $1"},
		{`"we?ird" = ? AND x = 'it''s?' AND y = ?`, `"we?ird" = $1 AND x = 'it''s?' AND y = $2`},
		{"shares @> jsonb_build_array(?::text)", "shares @> jsonb_build_array($1::text)"},
	}
	for _, tt := range tests {
		if got := rebind(tt.in); got != tt.want {
			t.Errorf("rebind(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
