//go:build integration

package suites

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// Calls that wait on a space's agent may take as long as the work, not the
// few seconds a client or the server allows an ordinary call: a command run
// with a long timeout, a script, and a script's own calls back to the API.
func TestSpaceLongCalls(t *testing.T) {
	harness.Feature(t, "run-command")
	id := workspace(t)
	ctx, cancel := testCtx(300)
	defer cancel()

	// A client as the knot command makes one, with its ten second timeout.
	client, err := apiclient.NewClient(server.BaseURL, user1.Token, true)
	if err != nil {
		t.Fatal(err)
	}

	out, err := client.RunCommand(ctx, id, &apiclient.RunCommandRequest{Command: "sleep 12; echo slept", Timeout: 30})
	if err != nil || !strings.Contains(out, "slept") {
		t.Errorf("a 12 second command: %q %v", out, err)
	}

	out, exit, err := client.ExecuteScriptContent(ctx, id, "import time\ntime.sleep(12)\nprint(\"slept\")\n", nil)
	if err != nil || exit != 0 || !strings.Contains(out, "slept") {
		t.Errorf("a 12 second script: %q exit %d %v", out, exit, err)
	}

	// A script's own call to run a command, past the ten seconds the
	// server's client for scripts waits by default.
	space, _, err := client.GetSpace(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	script := "import knot.space\nr = knot.space.run(\"" + space.Name + "\", \"sleep 12; echo inner\", timeout=30)\nprint(r)\n"
	out, exit, err = client.ExecuteScriptContent(ctx, id, script, nil)
	if err != nil || exit != 0 || !strings.Contains(out, "inner") {
		t.Errorf("knot.space.run of a 12 second command from a script: %q exit %d %v", out, exit, err)
	}
}
