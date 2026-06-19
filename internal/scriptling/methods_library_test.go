package scriptling

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/paularlott/knot/internal/methods"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/object"
)

// evalServer runs a snippet that constructs a Server and returns the
// Registration that was passed to methodsRegistrar.
func evalServer(t *testing.T, script string) (*methods.Registration, error) {
	t.Helper()

	var captured methods.Registration
	var mu sync.Mutex
	SetMethodsRegistrar(func(reg *methods.Registration) error {
		mu.Lock()
		defer mu.Unlock()
		captured = *reg
		return nil
	})
	defer SetMethodsRegistrar(nil)

	env := scriptling.New()
	env.RegisterLibrary(GetMethodsLibrary())
	env.RegisterLibrary(GetMethodsSchemaLibrary())
	full := "from knot.methods import Server, schema as s\n" + script
	res, err := env.EvalWithContext(context.Background(), full)
	if err != nil {
		return nil, err
	}
	if obj, ok := res.(*object.Error); ok {
		return nil, fmt.Errorf("%s", obj.Message)
	}
	mu.Lock()
	defer mu.Unlock()
	if captured.Server.Command == "" {
		return nil, nil
	}
	return &captured, nil
}

func TestServerBasicRegistration(t *testing.T) {
	reg, err := evalServer(t, `
server = Server("./bin/notes-rpc", timeout=30)
server.method(
    name="{{space}}.search",
    local_name="search",
    description="Search indexed notes in this space",
    keywords=["notes", "search", "documents"],
    scope="private",
    groups=[],
    mcp_tool=True,
    params=s.object(
        query=s.string(),
        tag=s.string(),
        limit=s.optional(s.integer(), default=10),
    ),
    result=s.object(),
)
ok = server.register()
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg == nil {
		t.Fatal("expected registration to be captured")
	}
	if reg.Server.Type != "stdio" {
		t.Errorf("server type: got %q", reg.Server.Type)
	}
	if reg.Server.Command != "./bin/notes-rpc" {
		t.Errorf("server command: got %q", reg.Server.Command)
	}
	if reg.Server.Timeout != 30 {
		t.Errorf("server timeout: got %d", reg.Server.Timeout)
	}
	if reg.Server.Mode != methods.ModeConcurrent {
		t.Errorf("server mode default: got %q", reg.Server.Mode)
	}
	if len(reg.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(reg.Methods))
	}
	m := reg.Methods[0]
	if m.Name != "{{space}}.search" {
		t.Errorf("method name: got %q", m.Name)
	}
	if m.LocalName != "search" {
		t.Errorf("method local_name: got %q", m.LocalName)
	}
	if !m.MCPTool {
		t.Errorf("method mcp_tool should be true")
	}
	if m.Scope != methods.ScopePrivate {
		t.Errorf("method scope: got %q", m.Scope)
	}
	if len(m.Keywords) != 3 || m.Keywords[0] != "notes" {
		t.Errorf("method keywords: got %v", m.Keywords)
	}
	if m.ParamsSchema == nil {
		t.Fatal("method params schema missing")
	}
	props, ok := m.ParamsSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("params properties missing: %#v", m.ParamsSchema)
	}
	if _, exists := props["query"]; !exists {
		t.Errorf("missing query property in params")
	}
	if _, exists := props["limit"]; !exists {
		t.Errorf("missing limit property in params")
	}
}

func TestServerModeKwarg(t *testing.T) {
	reg, err := evalServer(t, `
server = Server("./bin/slow", mode="serial")
server.method(name="ping", description="ping")
server.register()
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.Server.Mode != methods.ModeSerial {
		t.Errorf("expected serial mode, got %q", reg.Server.Mode)
	}
}

func TestServerRejectsUnknownKwarg(t *testing.T) {
	_, err := evalServer(t, `
server = Server("./bin/x", unknown=1)
server.method(name="ping", description="ping")
server.register()
`)
	if err == nil {
		t.Fatal("expected error for unknown Server kwarg")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error should mention unknown kwarg, got: %v", err)
	}
}

func TestServerRejectsUnknownMethodKwarg(t *testing.T) {
	_, err := evalServer(t, `
server = Server("./bin/x")
server.method(name="ping", description="ping", bogus=True)
server.register()
`)
	if err == nil {
		t.Fatal("expected error for unknown method() kwarg")
	}
}

func TestServerRegisterNotAvailable(t *testing.T) {
	// In an environment without methodsRegistrar set, register() errors.
	SetMethodsRegistrar(nil)

	env := scriptling.New()
	env.RegisterLibrary(GetMethodsLibrary())
	_, err := env.EvalWithContext(context.Background(), `
from knot.methods import Server
server = Server("./bin/x")
result = server.register()
`)
	if err == nil {
		t.Fatal("expected error when registrar is not set")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("expected 'not available' error, got: %v", err)
	}
}

func TestServerArgsKwarg(t *testing.T) {
	reg, err := evalServer(t, `
server = Server("scriptling", args=["./notes.sl"])
server.method(name="ping", description="ping")
server.register()
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reg.Server.Args) != 1 || reg.Server.Args[0] != "./notes.sl" {
		t.Errorf("expected args=[./notes.sl], got %v", reg.Server.Args)
	}
}

func TestServerDefaultsTypeToStdio(t *testing.T) {
	// `type` is optional and defaults to "stdio". Verified by registering with
	// no explicit type and inspecting the captured Registration.
	reg, err := evalServer(t, `
server = Server("./bin/notes-rpc")
server.method(name="ping", description="ping")
server.register()
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.Server.Type != methods.ServerTypeStdio {
		t.Errorf("expected default type %q, got %q", methods.ServerTypeStdio, reg.Server.Type)
	}
}

func TestServerTypeKwarg(t *testing.T) {
	// `type` can still be passed explicitly (forward-compat for future transports).
	reg, err := evalServer(t, `
server = Server("./bin/notes-rpc", type="stdio")
server.method(name="ping", description="ping")
server.register()
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.Server.Type != methods.ServerTypeStdio {
		t.Errorf("expected type %q, got %q", methods.ServerTypeStdio, reg.Server.Type)
	}
}
