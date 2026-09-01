package harness

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
)

var reaperWG sync.WaitGroup

func execDocker(args ...string) (string, error) {
	out, err := exec.Command("docker", args...).CombinedOutput()
	return string(out), err
}

// CreateSpace creates a space for the user and kicks off deployment
// (creation only persists the record; /start launches the container).
func CreateSpace(t *testing.T, client *apiclient.ApiClient, name, templateId, userId string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, code, err := client.CreateSpace(ctx, &apiclient.SpaceRequest{
		Name:       name,
		TemplateId: templateId,
		UserId:     userId,
		Shell:      "bash",
	})
	if err != nil {
		t.Fatalf("create space %s: %v (status %d)", name, err, code)
	}

	if code, err := client.StartSpace(ctx, id); err != nil {
		t.Fatalf("start space %s (%s): %v (status %d)", name, id, err, code)
	}
	return id
}

// WaitForSpaceReady blocks until the space's agent has registered and accepts
// commands. First boot includes pulling the image (pre-pulled by the harness)
// and the container entrypoint fetching + starting the agent.
func WaitForSpaceReady(t *testing.T, s *Server, client *apiclient.ApiClient, spaceId string) {
	t.Helper()
	timeout := time.Duration(s.Config.SpaceReadyTimeoutSeconds) * time.Second
	deadline := time.Now().Add(timeout)
	start := time.Now()

	t.Logf("waiting for space %s to become ready (timeout %s)", spaceId, timeout)
	Progress(fmt.Sprintf("waiting for space %s agent (booting container)", spaceId))

	// Phase 1: space deployed and agent state received.
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		space, _, err := client.GetSpace(ctx, spaceId)
		cancel()
		if err == nil && space != nil && space.IsDeployed && space.HasState && !space.IsPending {
			break
		}
		if err != nil {
			t.Logf("waiting for space %s: %v", spaceId, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("space %s deployed after %.1fs", spaceId, time.Since(start).Seconds())

	// Phase 2: the agent answers commands.
	for attempt := 0; time.Now().Before(deadline); {
		out, err := TryRunCommand(client, spaceId, 60, "echo", "knot-ready")
		if err == nil && strings.TrimSpace(out) == "knot-ready" {
			t.Logf("space %s ready in %.1fs", spaceId, time.Since(start).Seconds())
			return
		}
		attempt++
		if attempt%5 == 0 {
			remaining := time.Until(deadline).Truncate(time.Second)
			t.Logf("space %s agent not answering yet (err=%v out=%q, %s left)", spaceId, err, out, remaining)
			Progress(fmt.Sprintf("waiting for space %s agent to answer commands (%s left)", spaceId, remaining))
		}
		time.Sleep(1 * time.Second)
	}

	t.Fatalf("space %s never became ready within %s\nserver log tail:\n%s\n%s",
		spaceId, timeout, s.LogTail(60), containerLogs(spaceId))
}

// containerLogs dumps the docker container logs for a space (container name
// is <username>-<spacename>) to surface entrypoint/agent failures.
func containerLogs(spaceId string) string {
	nameOut, err := execDocker("ps", "-a", "--filter", "label=space_id="+spaceId, "--format", "{{.Names}}")
	if err != nil || strings.TrimSpace(nameOut) == "" {
		// fall back to any recent knot containers
		nameOut, _ = execDocker("ps", "-a", "--format", "{{.Names}}")
	}
	names := strings.Fields(nameOut)
	if len(names) == 0 {
		return "(no containers found)"
	}
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "--- docker logs %s ---\n", n)
		out, err := execDocker("logs", "--tail", "50", n)
		if err != nil {
			fmt.Fprintf(&b, "(error: %v)\n", err)
			continue
		}
		b.WriteString(out)
	}
	return b.String()
}

// RunCommand executes a command in the space and fails the test on error.
func RunCommand(t *testing.T, client *apiclient.ApiClient, spaceId string, timeout int, command string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout+30)*time.Second)
	defer cancel()

	out, err := client.RunCommand(ctx, spaceId, &apiclient.RunCommandRequest{
		Command: command,
		Args:    args,
		Timeout: timeout,
	})
	if err != nil {
		t.Fatalf("run-command %s %v in %s: %v (output: %s)", command, args, spaceId, err, out)
	}
	return out
}

// TryRunCommand is RunCommand without t.Fatalf; returns the error instead.
func TryRunCommand(client *apiclient.ApiClient, spaceId string, timeout int, command string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout+30)*time.Second)
	defer cancel()
	return client.RunCommand(ctx, spaceId, &apiclient.RunCommandRequest{
		Command: command,
		Args:    args,
		Timeout: timeout,
	})
}

// DeleteSpaceAsync schedules stop+delete to run in the background when the
// test finishes. Space teardown is dominated by the server's async
// container stop (tens of seconds); running it concurrently instead of
// blocking the test keeps per-test overhead at ~0s. TestMain waits for all
// reapers (WaitForSpaceReapers) before the final sweep.
func DeleteSpaceAsync(t *testing.T, client *apiclient.ApiClient, spaceId string) {
	t.Helper()
	t.Cleanup(func() {
		reaperWG.Add(1)
		go func() {
			defer reaperWG.Done()
			stopAndDelete(context.Background(), client, spaceId)
		}()
	})
}

// WaitForSpaceReapers blocks until all background teardowns finish (or the
// timeout elapses).
func WaitForSpaceReapers(timeout time.Duration) {
	done := make(chan struct{})
	go func() { reaperWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func stopAndDelete(ctx context.Context, client *apiclient.ApiClient, spaceId string) {
	client.StopSpace(ctx, spaceId)
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		c, cancel := context.WithTimeout(ctx, 15*time.Second)
		space, _, err := client.GetSpace(c, spaceId)
		cancel()
		if err != nil || space == nil || (!space.IsDeployed && !space.IsPending) {
			break
		}
		time.Sleep(1 * time.Second)
	}
	dctx, dcancel := context.WithTimeout(ctx, 30*time.Second)
	client.DeleteSpace(dctx, spaceId)
	dcancel()
}

// DeleteSpaceAndWait stops the space (the API refuses to delete a deployed
// space with 423) and then removes it, waiting until it disappears. A space
// already on the deletion path (is_deleting — e.g. removed by a stack delete)
// is left alone: deletion is async server-side and the record stays
// retrievable for a while, so is_deleting is the "done touching it" signal.
func DeleteSpaceAndWait(t *testing.T, client *apiclient.ApiClient, spaceId string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	space, _, err := client.GetSpace(ctx, spaceId)
	cancel()
	if err != nil {
		return // already gone
	}
	if space != nil && space.IsDeleting {
		waitForSpaceDeleting(t, client, spaceId)
		return
	}
	StopSpaceAndWait(t, client, spaceId)

	dctx, dcancel := context.WithTimeout(context.Background(), 120*time.Second)
	if code, err := client.DeleteSpace(dctx, spaceId); err != nil {
		t.Logf("delete space %s: %v (status %d) — continuing", spaceId, err, code)
	}
	dcancel()

	waitForSpaceDeleting(t, client, spaceId)
}

// waitForSpaceDeleting blocks until the space is gone (404) or marked
// is_deleting with nothing running.
func waitForSpaceDeleting(t *testing.T, client *apiclient.ApiClient, spaceId string) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		space, _, err := client.GetSpace(ctx, spaceId)
		cancel()
		if err != nil {
			return // gone
		}
		if space != nil && space.IsDeleting && !space.IsDeployed && !space.IsPending {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("space %s still listed after delete window (continuing)", spaceId)
}

// StopSpaceAndWait stops the space and waits for it to leave the deployed
// state.
func StopSpaceAndWait(t *testing.T, client *apiclient.ApiClient, spaceId string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	if code, err := client.StopSpace(ctx, spaceId); err != nil {
		t.Logf("stop space %s: %v (status %d) — continuing", spaceId, err, code)
	}
	cancel()

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		space, _, err := client.GetSpace(ctx, spaceId)
		cancel()
		if err != nil || space == nil || (!space.IsDeployed && !space.IsPending) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}
