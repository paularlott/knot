package model

import (
	"strings"
	"testing"
)

// API tokens carry a prefix, so none can begin with "-" and be taken for a
// command-line flag.
func TestNewTokenPrefix(t *testing.T) {
	for i := 0; i < 2000; i++ {
		id := NewToken("t", "user").Id
		if !strings.HasPrefix(id, TokenPrefix) || len(id) > 64 {
			t.Fatalf("token %q", id)
		}
	}
}
