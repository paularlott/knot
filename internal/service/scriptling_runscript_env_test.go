package service

import (
	"io"
	"strings"
	"testing"

	"github.com/paularlott/logger"
)

// evalImport attempts `import <name>` in a fresh run-script eval environment.
func evalImport(t *testing.T, name string) error {
	t.Helper()
	env, cleanup, err := NewAgentScriptlingEnv(nil, "", AgentScriptlingOptions{Argv: []string{"test"}, Logger: logger.NewNullLogger(), Output: io.Discard})
	if err != nil {
		t.Fatalf("NewAgentScriptlingEnv() failed: %v", err)
	}
	defer cleanup()
	_, err = env.Eval("import " + name)
	return err
}

// TestRunScriptEvalEnv_ContainerAndNomadUnavailable verifies the run-script
// eval environment never registers the container or nomad libraries — spaces
// manage containers through knot.space, never the runtime directly.
func TestRunScriptEvalEnv_ContainerAndNomadUnavailable(t *testing.T) {
	for _, lib := range []string{"scriptling.container", "scriptling.nomad"} {
		if err := evalImport(t, lib); err == nil {
			t.Errorf("import %s succeeded in run-script eval env, want unavailable", lib)
		}
	}
}

// TestRunScriptEvalEnv_LibrariesPresent pins knot's run-script library list:
// representative members of every group registerRunScriptLibraries registers
// must import cleanly.
func TestRunScriptEvalEnv_LibrariesPresent(t *testing.T) {
	libs := []string{
		// data / text
		"yaml", "toml", "scriptling.csv", "scriptling.xml",
		"scriptling.template.html", "scriptling.template.text", "shlex",
		// network
		"requests", "scriptling.wait_for",
		// system
		"subprocess", "os", "pathlib", "sys", "glob", "tempfile", "shutil",
		"zipfile", "tarfile", "fs", "scriptling.grep", "scriptling.find", "scriptling.sed",
		// ai / mcp / messaging
		"scriptling.ai", "scriptling.ai.agent", "scriptling.ai.tools", "scriptling.ai.memory",
		"scriptling.mcp", "scriptling.toon", "scriptling.mcp.tool",
		"scriptling.messaging.telegram", "scriptling.messaging.discord",
		"scriptling.messaging.slack",
		// misc
		"scriptling.similarity",
		"scriptling.provision.file", "scriptling.provision.fetch", "logging",
	}
	for _, lib := range libs {
		if err := evalImport(t, lib); err != nil {
			if strings.Contains(err.Error(), "already") {
				continue // library registered under a parent import — fine
			}
			t.Errorf("import %s failed in run-script eval env: %v", lib, err)
		}
	}
}

// TestAgentScriptlingEnv_NoServingSubLibs verifies the serving runtime
// sub-libraries are absent from the agent environment — serving belongs to the
// real scriptling CLI in the space, not the embedded eval runtime.
func TestAgentScriptlingEnv_NoServingSubLibs(t *testing.T) {
	env, cleanup, err := NewAgentScriptlingEnv(nil, "", AgentScriptlingOptions{})
	if err != nil {
		t.Fatalf("NewAgentScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	for _, lib := range []string{
		"scriptling.runtime",                       // the whole runtime family — real scriptling CLI only
		"scriptling.container", "scriptling.nomad", // container ops go via knot.space
		"scriptling.secret",      // no secret providers on the agent
		"scriptling.net.resolve", // resolver wired into requests/wait_for instead
		"scriptling.net.multicast", "scriptling.net.unicast", "scriptling.net.gossip",
		"scriptling.net.websocket", "scriptling.markdown",
		"scriptling.console", "scriptling.messaging.console",
		"scriptling.plugin",
	} {
		if _, err := env.Eval("import " + lib); err == nil {
			t.Errorf("import %s succeeded in agent env, want unavailable (serving is real-scriptling only)", lib)
		}
	}
}

// TestAgentScriptlingEnv_SharedSurface pins the consolidation: every
// agent-side context (startup scripts, streaming, run-script, health checks,
// methods registration) gets the same environment — including knot.healthcheck
// and the full extended library set — since NewAgentScriptlingEnv is the single
// constructor for all of them.
func TestAgentScriptlingEnv_SharedSurface(t *testing.T) {
	env, cleanup, err := NewAgentScriptlingEnv(nil, "", AgentScriptlingOptions{})
	if err != nil {
		t.Fatalf("NewAgentScriptlingEnv() failed: %v", err)
	}
	defer cleanup()

	for _, lib := range []string{"requests", "knot.healthcheck", "scriptling.messaging.telegram"} {
		if _, err := env.Eval("import " + lib); err != nil {
			t.Errorf("import %s failed in agent env: %v", lib, err)
		}
	}
}
