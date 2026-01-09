package service

import (
	"strings"
	"testing"

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

	env, err := NewMCPScriptlingEnv(nil, nil, user)
	if err != nil {
		t.Fatalf("NewMCPScriptlingEnv() failed: %v", err)
	}

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

	env, err := NewMCPScriptlingEnv(nil, nil, user)
	if err != nil {
		t.Fatalf("NewMCPScriptlingEnv() failed: %v", err)
	}

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

	env, err := NewMCPScriptlingEnv(nil, nil, user)
	if err != nil {
		t.Fatalf("NewMCPScriptlingEnv() failed: %v", err)
	}

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

	env, err := NewMCPScriptlingEnv(nil, nil, user)
	if err != nil {
		t.Fatalf("NewMCPScriptlingEnv() failed: %v", err)
	}

	// Try to import threads - should fail
	scriptContent := `
import threads
def background_task():
    pass
threads.start(background_task)
`

	_, err = env.Eval(scriptContent)
	if err == nil {
		t.Error("Expected error when importing threads in MCP environment, got nil")
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

	env, err := NewMCPScriptlingEnv(nil, nil, user)
	if err != nil {
		t.Fatalf("NewMCPScriptlingEnv() failed: %v", err)
	}

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

	env, err := NewMCPScriptlingEnv(nil, nil, user)
	if err != nil {
		t.Fatalf("NewMCPScriptlingEnv() failed: %v", err)
	}

	safeImports := []struct {
		name    string
		script  string
		verify  func(*testing.T, *scriptling.Scriptling, error)
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
import wait_for
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

// TestLocalScriptlingEnv_CanImportSystemLibraries verifies that local environment
// CAN import system libraries (contrast with MCP).
func TestLocalScriptlingEnv_CanImportSystemLibraries(t *testing.T) {
	env, err := NewLocalScriptlingEnv(nil, nil, "")
	if err != nil {
		t.Fatalf("NewLocalScriptlingEnv() failed: %v", err)
	}

	// These imports should succeed in local environment
	scriptContent := `
import subprocess
import os
import pathlib
import threads
import sys
result = "all_system_libs_imported"
`

	_, err = env.Eval(scriptContent)
	if err != nil {
		t.Errorf("Local environment should be able to import system libraries: %v", err)
	}
}

// TestRemoteScriptlingEnv_CanImportSystemLibraries verifies that remote environment
// CAN import system libraries (contrast with MCP).
func TestRemoteScriptlingEnv_CanImportSystemLibraries(t *testing.T) {
	env, err := NewRemoteScriptlingEnv(nil, nil, "")
	if err != nil {
		t.Fatalf("NewRemoteScriptlingEnv() failed: %v", err)
	}

	// These imports should succeed in remote environment
	scriptContent := `
import subprocess
import os
import pathlib
import threads
import sys
result = "all_system_libs_imported"
`

	_, err = env.Eval(scriptContent)
	if err != nil {
		t.Errorf("Remote environment should be able to import system libraries: %v", err)
	}
}
