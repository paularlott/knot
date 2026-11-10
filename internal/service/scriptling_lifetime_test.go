package service

import (
	"io"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/logger"
	"github.com/paularlott/scriptling"
)

const openClient = `import scriptling.mercure as mercure
hub = mercure.Client("http://127.0.0.1:9/.well-known/mercure")
`

// holdClient makes a Mercure client in env and returns it, so it can be
// looked at after the env's bindings are gone.
func holdClient(t *testing.T, env *scriptling.Scriptling) func() bool {
	t.Helper()
	if _, err := env.Eval(openClient); err != nil {
		t.Fatal(err)
	}
	hub, err := env.GetVarAsObject("hub")
	if err != nil {
		t.Fatal(err)
	}
	// closed reports whether the client closes within a second (it is
	// closed on a goroutine of its own when its lifetime ends).
	return func() bool {
		for deadline := time.Now().Add(time.Second); ; time.Sleep(10 * time.Millisecond) {
			if err := env.SetObjectVar("held", hub); err != nil {
				t.Fatal(err)
			}
			v, err := env.Eval("held.closed")
			if err != nil {
				t.Fatal(err)
			}
			if v.Inspect() == "True" || time.Now().After(deadline) {
				return v.Inspect() == "True"
			}
		}
	}
}

// A server env's cleanup closes the clients its script left open.
func TestServerEnvCleanupClosesMercure(t *testing.T) {
	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	closed := holdClient(t, env)
	if v, _ := env.Eval("hub.closed"); v.Inspect() != "False" {
		t.Fatal("closed before cleanup")
	}
	cleanup()
	if !closed() {
		t.Fatal("still open after cleanup")
	}
}

// A pooled agent env is reset between uses: what one run left open goes,
// and the next run can make clients again.
func TestAgentEnvResetClosesMercure(t *testing.T) {
	env, cleanup, err := NewAgentScriptlingEnv(nil, "", AgentScriptlingOptions{Logger: logger.NewNullLogger(), Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	closed := holdClient(t, env)
	env.Reset()
	if !closed() {
		t.Fatal("still open after reset")
	}
	if _, err := env.Eval(openClient + "hub.closed"); err != nil {
		t.Fatalf("a client after the reset: %v", err)
	}
}

// Releasing a plugin env ends the lease and with it the handler's clients.
func TestPluginEnvReleaseClosesMercure(t *testing.T) {
	defer ResetPluginEnvPoolForTest()
	p := &plugins.Plugin{}
	env := buildPluginEnv(p)
	closed := holdClient(t, env)
	ReleasePluginEnv(env, p)
	if !closed() {
		t.Fatal("still open after release")
	}
}

// The agent env can run provisioning steps as root; mercure is everywhere.
func TestProvisionAndMercureRegistered(t *testing.T) {
	for _, lib := range []string{"scriptling.provision.command", "scriptling.provision.package", "scriptling.mercure"} {
		if err := evalImport(t, lib); err != nil {
			t.Errorf("agent env: import %s: %v", lib, err)
		}
	}
	env, _, cleanup, err := NewServerScriptlingEnv(nil, ServerScriptlingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := env.Eval("import scriptling.mercure"); err != nil {
		t.Errorf("server env: %v", err)
	}
	for _, lib := range []string{"scriptling.provision.command", "scriptling.provision.file"} {
		if _, err := env.Eval("import " + lib); err == nil {
			t.Errorf("server env imports %s: provisioning is agent only", lib)
		}
	}
}
