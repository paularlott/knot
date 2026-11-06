//go:build integration

package suites

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// TestSpaceAgentLogs checks where a space's agent and its startup script
// write: what the startup script prints, and the agent's own records, reach
// both the container's log, as they are and once each, and the space's log
// window.
func TestSpaceAgentLogs(t *testing.T) {
	harness.Feature(t, "agent-logs")
	runtime := cfg.Runtime
	if runtime != "docker" && runtime != "podman" {
		t.Skipf("needs docker or podman to read the container's log, not %s", runtime)
	}
	s, admin := bootDedicated(t, "agentlogs")
	ctx, cancel := testCtx(300)
	defer cancel()

	const printed = "printed by the startup script"
	script, err := admin.Client.CreateScript(ctx, apiclient.ScriptCreateRequest{
		UserId:     admin.Id,
		Name:       "it_startup_logs",
		Content:    "print(\"" + printed + "\")\nprint(\"and a second line\")\n",
		Active:     true,
		ScriptType: "script",
	})
	if err != nil {
		t.Fatalf("create script: %v", err)
	}
	tmpl, err := harness.CreateTemplate(s, admin.Client, uniqueName("it_agent_logs"), harness.TemplateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	space, code, err := admin.Client.CreateSpace(ctx, &apiclient.SpaceRequest{
		Name:            "it-agent-logs",
		TemplateId:      tmpl,
		UserId:          admin.Id,
		Shell:           "bash",
		StartupScriptId: script.Id,
	})
	if err != nil {
		t.Fatalf("create space: %v (status %d)", err, code)
	}
	harness.DeleteSpaceAsync(t, admin.Client, space)
	if code, err := admin.Client.StartSpace(ctx, space); err != nil {
		t.Fatalf("start space: %v (status %d)", err, code)
	}
	harness.WaitForSpaceReady(t, s, admin.Client, space)

	// A script that fails has the agent log an error of its own.
	const failure = "it-agent-logs-failure"
	if _, _, err := admin.Client.ExecuteScriptContent(ctx, space, "raise Exception(\""+failure+"\")\n", nil); err != nil {
		t.Logf("the failing script: %v", err)
	}

	containerLog := func() string {
		out, _ := exec.Command(runtime, "logs", spaceContainer(t, runtime, space)).CombinedOutput()
		return string(out)
	}
	var window, container string
	if !waitForCond(60, func() bool {
		window, container = strings.Join(spaceLogLines(t, admin, space), "\n"), containerLog()
		return strings.Contains(window, printed) && strings.Contains(window, failure) &&
			strings.Contains(container, printed) && strings.Contains(container, failure)
	}) {
		t.Fatalf("the records did not reach both logs\nspace log:\n%s\ncontainer log:\n%s", window, container)
	}
	if !strings.Contains(window, "and a second line") || !strings.Contains(container, "and a second line") {
		t.Errorf("the startup script's second line is missing")
	}
	// The script's output reaches the container's log as it was printed.
	if !strings.Contains(container, "\n"+printed+"\n") && !strings.HasPrefix(container, printed+"\n") {
		t.Errorf("the startup script's output was changed on its way to the container's log:\n%s", container)
	}
	if n := strings.Count(container, failure); n != 1 {
		t.Errorf("the agent's error is in the container's log %d times, want once:\n%s", n, container)
	}
}

// spaceLogLines is the space's log window as it is now.
func spaceLogLines(t *testing.T, u *harness.User, space string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ws, _, err := wsDial(ctx, u.Token, u.Client.GetWebSocketURL()+"/logs/"+space+"/stream")
	if err != nil {
		t.Logf("no log stream: %v", err)
		return nil
	}
	defer ws.Close()
	var lines []string
	func() {
		defer func() { recover() }()
		for {
			ws.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			lines = append(lines, string(data))
		}
	}()
	return lines
}
