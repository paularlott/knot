package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/scriptling"
)

// TestMCPScriptlingEnv_CannotImportSubprocess verifies that attempting
// to import subprocess in MCP environment fails.
func TestMCPScriptlingEnv_CannotImportSubprocess(t *testing.T) {
	user := &model.User{
		Id:       "test-user",
		Username: "testuser",
		Email:    "test@example.com",
	}

	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	// Try to import subprocess - should fail
	scriptContent := `
import subprocess
result = subprocess.run(["ls", "-la"])
`

	_, err = env.Eval(scriptContent)
	if err == nil {
		t.Error("Expected error when importing subprocess in MCP environment, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "not found") &&
		!strings.Contains(strings.ToLower(err.Error()), "unknown") &&
		!strings.Contains(strings.ToLower(err.Error()), "no module") {
		t.Logf("Got expected error (may vary): %v", err)
	}
}

// TestMCPScriptlingEnv_CannotImportOS verifies that attempting
// to import os in MCP environment fails.
func TestMCPScriptlingEnv_CannotImportOS(t *testing.T) {
	user := &model.User{
		Id:       "test-user",
		Username: "testuser",
		Email:    "test@example.com",
	}

	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	// Try to import os - should fail
	scriptContent := `
import os
files = os.listdir("/")
`

	_, err = env.Eval(scriptContent)
	if err == nil {
		t.Error("Expected error when importing os in MCP environment, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "not found") &&
		!strings.Contains(strings.ToLower(err.Error()), "unknown") &&
		!strings.Contains(strings.ToLower(err.Error()), "no module") {
		t.Logf("Got expected error (may vary): %v", err)
	}
}

// TestMCPScriptlingEnv_CannotImportPathlib verifies that attempting
// to import pathlib in MCP environment fails.
func TestMCPScriptlingEnv_CannotImportPathlib(t *testing.T) {
	user := &model.User{
		Id:       "test-user",
		Username: "testuser",
		Email:    "test@example.com",
	}

	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	// Try to import pathlib - should fail
	scriptContent := `
import pathlib
p = pathlib.Path("/etc/passwd")
content = p.read_text()
`

	_, err = env.Eval(scriptContent)
	if err == nil {
		t.Error("Expected error when importing pathlib in MCP environment, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "not found") &&
		!strings.Contains(strings.ToLower(err.Error()), "unknown") &&
		!strings.Contains(strings.ToLower(err.Error()), "no module") {
		t.Logf("Got expected error (may vary): %v", err)
	}
}

// TestMCPScriptlingEnv_CannotImportThreads verifies that attempting
// to import threads in MCP environment fails.
func TestMCPScriptlingEnv_CannotImportThreads(t *testing.T) {
	user := &model.User{
		Id:       "test-user",
		Username: "testuser",
		Email:    "test@example.com",
	}

	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	// Try to import threads - should fail
	scriptContent := `
import scriptling.threads
def background_task():
    pass
scriptling.threads.run(background_task)
`

	_, err = env.Eval(scriptContent)
	if err == nil {
		t.Error("Expected error when importing scriptling.threads in MCP environment, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "not found") &&
		!strings.Contains(strings.ToLower(err.Error()), "unknown") &&
		!strings.Contains(strings.ToLower(err.Error()), "no module") {
		t.Logf("Got expected error (may vary): %v", err)
	}
}

// TestMCPScriptlingEnv_CannotImportSys verifies that attempting
// to import sys in MCP environment fails.
func TestMCPScriptlingEnv_CannotImportSys(t *testing.T) {
	user := &model.User{
		Id:       "test-user",
		Username: "testuser",
		Email:    "test@example.com",
	}

	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	// Try to import sys - should fail
	scriptContent := `
import sys
sys.argv.append("--malicious-flag")
`

	_, err = env.Eval(scriptContent)
	if err == nil {
		t.Error("Expected error when importing sys in MCP environment, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "not found") &&
		!strings.Contains(strings.ToLower(err.Error()), "unknown") &&
		!strings.Contains(strings.ToLower(err.Error()), "no module") {
		t.Logf("Got expected error (may vary): %v", err)
	}
}

// TestMCPScriptlingEnv_CanImportSafeLibraries verifies that safe libraries
// CAN be imported in the MCP environment.
func TestMCPScriptlingEnv_CanImportSafeLibraries(t *testing.T) {
	user := &model.User{
		Id:       "test-user",
		Username: "testuser",
		Email:    "test@example.com",
	}

	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	safeImports := []struct {
		name   string
		script string
		verify func(*testing.T, *scriptling.Scriptling, error)
	}{
		{
			name: "stdlib_builtins",
			script: `
# stdlib is the base runtime, not importable
# Test that basic Python builtins work
result = len([1, 2, 3])
`,
			verify: func(t *testing.T, env *scriptling.Scriptling, err error) {
				if err != nil {
					t.Errorf("Failed to use stdlib builtins: %v", err)
				}
			},
		},
		{
			name: "requests",
			script: `
import requests
# Just import, don't actually make a request
result = "requests_imported"
`,
			verify: func(t *testing.T, env *scriptling.Scriptling, err error) {
				if err != nil {
					t.Errorf("Failed to import requests: %v", err)
				}
			},
		},
		{
			name: "secrets",
			script: `
import secrets
token = secrets.token_hex(8)
result = len(token) == 16
`,
			verify: func(t *testing.T, env *scriptling.Scriptling, err error) {
				if err != nil {
					t.Errorf("Failed to import secrets: %v", err)
				}
			},
		},
		{
			name: "html_parser",
			script: `
import html.parser
html = "<div>Hello</div>"
# Just import, don't parse
result = "htmlparser_imported"
`,
			verify: func(t *testing.T, env *scriptling.Scriptling, err error) {
				if err != nil {
					t.Errorf("Failed to import html.parser: %v", err)
				}
			},
		},
		{
			name: "wait_for",
			script: `
import scriptling.wait_for
result = "wait_for_imported"
`,
			verify: func(t *testing.T, env *scriptling.Scriptling, err error) {
				if err != nil {
					t.Errorf("Failed to import wait_for: %v", err)
				}
			},
		},
	}

	for _, tc := range safeImports {
		t.Run("safe_"+tc.name, func(t *testing.T) {
			_, err := env.Eval(tc.script)
			tc.verify(t, env, err)
		})
	}
}

// TestRemoteScriptlingEnv_CanImportSystemLibraries verifies that remote environment
// CAN import system libraries (contrast with MCP).
func TestRemoteScriptlingEnv_CanImportSystemLibraries(t *testing.T) {
	env, cleanup, err := NewAgentScriptlingEnv(nil, "", AgentScriptlingOptions{})
	if err != nil {
		t.Fatalf("NewAgentScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	// These imports should succeed in remote environment
	scriptContent := `
import subprocess
import os
import pathlib
import sys
result = "all_system_libs_imported"
`

	_, err = env.Eval(scriptContent)
	if err != nil {
		t.Errorf("Remote environment should be able to import system libraries: %v", err)
	}
}

func TestRemoteScriptlingEnv_CanImportProvisionFetch(t *testing.T) {
	env, cleanup, err := NewAgentScriptlingEnv(nil, "", AgentScriptlingOptions{Output: io.Discard})
	if err != nil {
		t.Fatalf("NewAgentScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	_, err = env.Eval(`
import scriptling.provision.fetch
result = "provision_fetch_imported"
`)
	if err != nil {
		t.Errorf("Remote environment should be able to import scriptling.provision.fetch: %v", err)
	}
}

// TestMCPScriptlingEnv_FSNotRegisteredByDefault verifies the fs library is not
// available in server-side environments unless ScriptFSAllowedPaths is set.
func TestMCPScriptlingEnv_FSNotRegisteredByDefault(t *testing.T) {
	user := &model.User{
		Id:       "test-user",
		Username: "testuser",
		Email:    "test@example.com",
	}

	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	if _, err := env.Eval("import fs"); err == nil {
		t.Error("Expected error importing fs in MCP environment with no ScriptFSAllowedPaths configured, got nil")
	}
}

// TestMCPScriptlingEnv_FSRegisteredWithConfig verifies the fs library is
// registered and path-restricted when ScriptFSAllowedPaths is configured.
func TestMCPScriptlingEnv_FSRegisteredWithConfig(t *testing.T) {
	dir := t.TempDir()
	orig := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{ScriptFSAllowedPaths: []string{dir}})
	t.Cleanup(func() { config.SetServerConfig(orig) })

	user := &model.User{
		Id:       "test-user",
		Username: "testuser",
		Email:    "test@example.com",
	}

	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	if _, err := env.Eval("import fs"); err != nil {
		t.Fatalf("import fs failed with ScriptFSAllowedPaths configured: %v", err)
	}

	// Reading outside the allowed path must fail.
	if _, err := env.Eval(`import fs
fs.read_bytes("/etc/passwd", 0, 4)`); err == nil {
		t.Error("Expected error reading outside allowed paths, got nil")
	}
}

// writeNetPolicy writes a scriptling network policy TOML file to a temp dir
// and returns its path.
func writeNetPolicy(t *testing.T, toml string) string {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/policy.toml"
	if err := os.WriteFile(path, []byte(toml), 0o644); err != nil {
		t.Fatalf("failed to write policy file: %v", err)
	}
	return path
}

// TestMCPScriptlingEnv_NetUnrestrictedByDefault verifies that requests made
// by server-side scripts are not restricted when ScriptNetPolicyFile is not
// configured (today's behaviour, preserved).
func TestMCPScriptlingEnv_NetUnrestrictedByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	user := &model.User{Id: "test-user", Username: "testuser", Email: "test@example.com"}
	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	if _, err := env.Eval("import requests\nrequests.get('" + srv.URL + "')"); err != nil {
		t.Errorf("unrestricted request should succeed, got: %v", err)
	}
}

// TestMCPScriptlingEnv_NetRestrictedByAllowHosts verifies that requests are
// restricted to the policy file's allow_hosts list — the network equivalent
// of TestMCPScriptlingEnv_FSRegisteredWithConfig.
func TestMCPScriptlingEnv_NetRestrictedByAllowHosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	// Use the "localhost" hostname rather than the literal 127.0.0.1 address
	// so the request goes through the host-allowlist check rather than the
	// (always denied by default) IP-literal check.
	hostURL := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)

	user := &model.User{Id: "test-user", Username: "testuser", Email: "test@example.com"}
	orig := config.GetServerConfig()
	t.Cleanup(func() { config.SetServerConfig(orig) })

	// Denied: localhost is not in the configured allow-list.
	config.SetServerConfig(&config.ServerConfig{
		ScriptNetPolicyFile: writeNetPolicy(t, `allow_hosts = ["allowed.example.com"]`),
	})

	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	if _, err := env.Eval("import requests\nrequests.get('" + hostURL + "')"); err == nil ||
		!strings.Contains(err.Error(), "not in the allowed host list") {
		t.Errorf("expected host-not-allowed error, got: %v", err)
	}

	// Allowed: localhost is explicitly listed.
	config.SetServerConfig(&config.ServerConfig{
		ScriptNetPolicyFile: writeNetPolicy(t, `allow_hosts = ["localhost"]`),
	})
	env2, _, cleanup2, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv() failed: %v", err)
	}
	defer cleanup2()

	if _, err := env2.Eval("import requests\nrequests.get('" + hostURL + "')"); err != nil {
		t.Errorf("allow-listed host should succeed, got: %v", err)
	}
}

// TestMCPScriptlingEnv_NetPolicyHTTPSOnly verifies that the policy file's
// https_only option is honoured, not just allow_hosts — the admin has the
// full scriptling network-policy schema available, not a single flag.
func TestMCPScriptlingEnv_NetPolicyHTTPSOnly(t *testing.T) {
	user := &model.User{Id: "test-user", Username: "testuser", Email: "test@example.com"}
	orig := config.GetServerConfig()
	t.Cleanup(func() { config.SetServerConfig(orig) })

	config.SetServerConfig(&config.ServerConfig{
		ScriptNetPolicyFile: writeNetPolicy(t, `https_only = true`),
	})

	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user})
	if err != nil {
		t.Fatalf("NewServerScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	if _, err := env.Eval("import requests\nrequests.get('http://example.com/')"); err == nil ||
		!strings.Contains(err.Error(), "requires https") {
		t.Errorf("expected https-only block, got: %v", err)
	}
}

// TestMCPScriptlingEnv_NetPolicyInvalidFailsClosed verifies that an invalid
// or missing policy file makes environment creation fail outright, rather
// than silently falling back to an unrestricted policy.
func TestMCPScriptlingEnv_NetPolicyInvalidFailsClosed(t *testing.T) {
	user := &model.User{Id: "test-user", Username: "testuser", Email: "test@example.com"}
	orig := config.GetServerConfig()
	t.Cleanup(func() { config.SetServerConfig(orig) })

	config.SetServerConfig(&config.ServerConfig{
		ScriptNetPolicyFile: writeNetPolicy(t, `allow_cidrs = ["not-a-cidr"]`),
	})

	if _, _, _, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user}); err == nil {
		t.Error("expected error creating environment with invalid script-net-policy, got nil")
	}

	config.SetServerConfig(&config.ServerConfig{ScriptNetPolicyFile: "/nonexistent/policy.toml"})
	if _, _, _, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{User: user}); err == nil {
		t.Error("expected error creating environment with missing script-net-policy file, got nil")
	}
}
