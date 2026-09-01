//go:build integration

package suites

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// TestRunScriptPaths verifies both run-script entry points after the eval-only
// rework: the server-dispatched execution used by `knot space run-script`
// (non-stream and stream backends) and the in-space agent CLI `knot
// run-script`. Streaming scripts print directly — no console/interact.
func TestRunScriptPaths(t *testing.T) {
	harness.Feature(t, "run-script-paths")

	ctx, cancel := testCtx(30)
	created, err := user1.Client.CreateScript(ctx, apiclient.ScriptCreateRequest{
		UserId:     user1.Id,
		Name:       "it_rsmark",
		Content:    "import requests\nprint(\"RSMARK\")\n",
		Active:     true,
		ScriptType: "script",
	})
	cancel()
	if err != nil {
		t.Fatalf("create script: %v", err)
	}
	defer func() {
		ctx, cancel := testCtx(15)
		user1.Client.DeleteScript(ctx, created.Id)
		cancel()
	}()

	id := spaceFixture(t, "it-rs-paths", user1.Id, user1.Client)

	// 1. Server-dispatched, non-streaming (POST execute-script).
	ectx, ecancel := testCtx(120)
	out, exitCode, err := user1.Client.ExecuteScript(ectx, id, created.Id, nil)
	ecancel()
	if err != nil || exitCode != 0 {
		t.Fatalf("execute-script: err=%v exit=%d output=%s", err, exitCode, out)
	}
	if !strings.Contains(out, "RSMARK") {
		t.Fatalf("execute-script output missing marker: %s", out)
	}

	// 2. Server-dispatched, streaming (execute-script-stream — the knot space
	// run-script backend). Output is mirrored to the test process stdout; the
	// exit code is the assertion.
	sctx, scancel := testCtx(120)
	exitCode, err = user1.Client.ExecuteScriptStream(sctx, id, "it_rsmark", nil)
	scancel()
	if err != nil || exitCode != 0 {
		t.Fatalf("execute-script-stream: err=%v exit=%d", err, exitCode)
	}

	// 3. Agent CLI: knot run-script <name> inside the space.
	cliOut := harness.RunCommand(t, user1.Client, id, 120, "/usr/local/bin/knot", "run-script", "it_rsmark")
	if !strings.Contains(cliOut, "RSMARK") {
		t.Fatalf("knot run-script output missing marker: %s", cliOut)
	}
}
