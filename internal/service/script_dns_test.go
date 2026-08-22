package service

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/dns"
)

// Knot's configured nameservers must serve scriptling's dial paths, not just
// scriptling.net.resolve. A dead nameserver proves requests resolves through
// the configured servers; with no nameservers configured nothing changes
// (system resolver, full access).
func TestScriptDialsUseConfiguredNameservers(t *testing.T) {
	// Preserve and restore the process-wide resolver configuration.
	defer dns.UpdateNameservers(nil)

	user := &model.User{Id: "test-user", Username: "testuser", Email: "test@example.com"}

	// No nameservers configured: full access via the system resolver.
	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv: %v", err)
	}
	if _, err := env.Eval("import requests\nresp = requests.get('http://localhost:1/', timeout=1)\nresp.status_code"); err == nil {
		cleanup()
		t.Fatal("expected connection error, got success")
	} else if strings.Contains(err.Error(), "network policy") {
		cleanup()
		t.Fatalf("no nameservers must mean no policy, got: %v", err)
	}
	cleanup()

	// Dead nameserver: the dial path must resolve through it.
	dns.UpdateNameservers([]string{"127.0.0.1:1"})
	env2, _, cleanup2, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv: %v", err)
	}
	defer cleanup2()
	_, err = env2.Eval("import requests\nresp = requests.get('http://not-in-hosts.test/', timeout=2)\nresp.status_code")
	if err == nil {
		t.Fatal("dead nameserver should fail the request")
	}
	if msg := err.Error(); strings.Contains(msg, "not allowed") {
		t.Errorf("resolver-only mode must keep full access, got: %v", err)
	}
}
