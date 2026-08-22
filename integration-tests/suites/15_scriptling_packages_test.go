//go:build integration

package suites

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// scriptlingImage is the Scriptling base image deployed for this test run.
const scriptlingImage = "docker.io/paularlott/knot-scriptling:0.21-alpine"

// runScriptling runs the real Scriptling CLI in the space with the agent's
// package endpoints wired in and returns its output.
func runScriptling(t *testing.T, client *apiclient.ApiClient, spaceId string, file string) string {
	t.Helper()
	return harness.RunCommand(t, client, spaceId, 180, "/usr/local/bin/scriptling",
		"--package", "http://127.0.0.1:12201/packages/knot.zip",
		"--package", "http://127.0.0.1:12201/packages/libs.zip",
		file,
	)
}

// TestScriptlingPackages verifies the in-space Scriptling flow end to end on a
// Scriptling base image: the agent serves knot.zip (knot.* libraries) and
// libs.zip (user + global lib scripts) as cached packages, knot.apiclient
// configures itself from the agent's /connect endpoint, the script calls the
// knot API as the space owner, and a library update on the server reaches the
// space via the change notification.
func TestScriptlingPackages(t *testing.T) {
	harness.Feature(t, "scriptling-packages")

	// A user library the in-space Scriptling imports from libs.zip.
	ctx, cancel := testCtx(30)
	created, err := user1.Client.CreateScript(ctx, apiclient.ScriptCreateRequest{
		UserId:     user1.Id,
		Name:       "ittestlib",
		Content:    "value = \"v1\"\n",
		Active:     true,
		ScriptType: "lib",
	})
	cancel()
	if err != nil {
		t.Fatalf("create lib script: %v", err)
	}
	defer func() {
		ctx, cancel := testCtx(15)
		user1.Client.DeleteScript(ctx, created.Id)
		cancel()
	}()

	// Template on the Scriptling base image; KNOT_API_PORT matches the agent
	// default so in-space processes can find the package endpoints.
	tmplId, err := harness.CreateTemplate(server, admin.Client, "it-scriptling", harness.TemplateOptions{
		Image:    scriptlingImage,
		ExtraEnv: []string{"KNOT_API_PORT=12201"},
	})
	if err != nil {
		t.Fatalf("create scriptling template: %v", err)
	}
	defer func() {
		ctx, cancel := testCtx(15)
		admin.Client.DeleteTemplate(ctx, tmplId)
		cancel()
	}()

	spaceId := harness.CreateSpace(t, user1.Client, "it-sl-pkg", tmplId, user1.Id)
	harness.DeleteSpaceAsync(t, admin.Client, spaceId)
	harness.WaitForSpaceReady(t, server, user1.Client, spaceId)

	// The probe script: imports the user lib (libs.zip), uses knot.user via
	// knot.apiclient (knot.zip + /connect auto-config + live API call).
	probe := `import ittestlib
import knot.user

me = knot.user.get_me()
print("LIB:" + ittestlib.value)
print("USER:" + me["username"])
print("SPACES:" + str(len(knot.user.list()) > 0))
`
	wctx, wcancel := testCtx(30)
	if err := user1.Client.WriteSpaceFile(wctx, spaceId, "/tmp/sltest.py", probe); err != nil {
		wcancel()
		t.Fatalf("write probe script: %v", err)
	}
	wcancel()

	out := runScriptling(t, user1.Client, spaceId, "/tmp/sltest.py")
	t.Logf("scriptling output:\n%s", out)

	if !strings.Contains(out, "LIB:v1") {
		t.Fatalf("user lib not loaded via agent libs.zip package: %s", out)
	}
	if !strings.Contains(out, "USER:"+user1.Username) {
		t.Fatalf("knot API call did not act as the space owner (got: %s)", out)
	}
	if !strings.Contains(out, "SPACES:True") {
		t.Fatalf("knot.user.list() call failed via /connect auto-config (got: %s)", out)
	}

	// Update the library on the server; the notification must reach the agent,
	// which drops its libs.zip cache — the next run sees v2.
	uctx, ucancel := testCtx(30)
	err = user1.Client.UpdateScript(uctx, created.Id, apiclient.ScriptUpdateRequest{
		Name:       "ittestlib",
		Content:    "value = \"v2\"\n",
		Active:     true,
		ScriptType: "lib",
	})
	ucancel()
	if err != nil {
		t.Fatalf("update lib script: %v", err)
	}

	deadline := time.Now().Add(45 * time.Second)
	updated := false
	for time.Now().Before(deadline) {
		out = runScriptling(t, user1.Client, spaceId, "/tmp/sltest.py")
		if strings.Contains(out, "LIB:v2") {
			updated = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !updated {
		t.Fatalf("library update did not reach the space via notification (last output:\n%s)", out)
	}
	fmt.Println("== scriptling packages: libs.zip, knot.zip, /connect and change notification verified ==")
}
