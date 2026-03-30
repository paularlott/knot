package docker

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/util/rest"
)

// candidateSockets lists socket paths to probe in order.
var candidateSockets = []string{
	"/var/run/docker.sock",
	"/run/docker.sock",
	"/Users/paul/.lima/docker/sock/docker.sock",
	"/run/user/1000/docker.sock",
}

// findDockerSocket returns the first reachable Docker socket path, or "".
func findDockerSocket() string {
	for _, path := range candidateSockets {
		conn, err := net.DialTimeout("unix", path, 2*time.Second)
		if err == nil {
			conn.Close()
			return path
		}
	}
	return ""
}

func newTestClient(t *testing.T) *DockerClient {
	t.Helper()
	sock := findDockerSocket()
	if sock == "" {
		t.Skip("Docker socket not available, skipping integration test")
	}
	hc, err := rest.NewUnixSocketClient("unix://" + sock)
	if err != nil {
		t.Fatalf("NewUnixSocketClient: %v", err)
	}
	return &DockerClient{httpClient: hc}
}

func TestIntegration_VolumeCreateRemove(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	const volName = "knot-test-volume-integration"

	// Clean up any leftover from a previous run
	_ = c.volumeRemove(ctx, volName)

	// Create
	name, err := c.volumeCreate(ctx, volName)
	if err != nil {
		t.Fatalf("volumeCreate: %v", err)
	}
	if name != volName {
		t.Errorf("volumeCreate returned name %q, want %q", name, volName)
	}

	// Idempotent create (Docker returns 201 for existing volume with same name)
	_, err = c.volumeCreate(ctx, volName)
	if err != nil {
		t.Errorf("volumeCreate (idempotent) unexpected error: %v", err)
	}

	// Remove
	if err := c.volumeRemove(ctx, volName); err != nil {
		t.Fatalf("volumeRemove: %v", err)
	}

	// Remove again — should be silent (not-found ignored)
	if err := c.volumeRemove(ctx, volName); err != nil {
		t.Errorf("volumeRemove (not-found) unexpected error: %v", err)
	}
}

func TestIntegration_ImagePull(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// hello-world is tiny (~13KB) — fast to pull
	if err := c.imagePull(ctx, "hello-world:latest", ""); err != nil {
		t.Fatalf("imagePull: %v", err)
	}

	// Non-existent tag should return an error from the stream
	err := c.imagePull(ctx, "hello-world:this-tag-does-not-exist-knot-test", "")
	if err == nil {
		t.Error("expected error pulling non-existent image tag, got nil")
	} else {
		t.Logf("correctly got error for bad tag: %v", err)
	}
}

func TestIntegration_ContainerStopTimeout(t *testing.T) {
	// Verifies that containerStop doesn't time out during the Docker grace period.
	// Runs a container with a process that ignores SIGTERM (sleep), so Docker
	// waits the full grace period before SIGKILL. The call must not error due
	// to the 10s HTTPClient timeout firing before Docker responds.
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := c.imagePull(ctx, "alpine:latest", ""); err != nil {
		t.Fatalf("imagePull: %v", err)
	}

	const containerName = "knot-test-stop-timeout"
	_ = c.containerStop(ctx, containerName)
	_ = c.containerRemove(ctx, containerName)

	// sleep 60 ignores SIGTERM, so Docker will wait the grace period (we pass t=12)
	id, err := c.containerCreate(ctx, containerName, containerCreateRequest{
		Image:    "alpine:latest",
		Hostname: "knot-test",
		Cmd:      []string{"sleep", "60"},
		HostConfig: containerHostConfig{
			RestartPolicy: restartPolicy{Name: "no"},
		},
	})
	if err != nil {
		t.Fatalf("containerCreate: %v", err)
	}
	if err := c.containerStart(ctx, id); err != nil {
		_ = c.containerRemove(ctx, id)
		t.Fatalf("containerStart: %v", err)
	}

	// Stop with a 12s grace period — this will block >10s, which would previously
	// trigger the HTTPClient timeout.
	stopCtx, stopCancel := context.WithTimeout(ctx, 30*time.Second)
	defer stopCancel()

	start := time.Now()
	if err := c.containerStop(stopCtx, id); err != nil {
		_ = c.containerRemove(ctx, id)
		t.Fatalf("containerStop: %v", err)
	}
	elapsed := time.Since(start)
	t.Logf("containerStop took %v", elapsed)

	_ = c.containerRemove(ctx, id)

	// Should have taken at least 10s (Docker default grace period)
	if elapsed < 10*time.Second {
		t.Logf("note: stop completed in %v (container may have exited cleanly)", elapsed)
	}
}

func TestIntegration_ContainerLifecycle(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Pull image first
	if err := c.imagePull(ctx, "hello-world:latest", ""); err != nil {
		t.Fatalf("imagePull: %v", err)
	}

	const containerName = "knot-test-container-integration"

	// Clean up any leftover
	_ = c.containerStop(ctx, containerName)
	_ = c.containerRemove(ctx, containerName)

	// Create
	id, err := c.containerCreate(ctx, containerName, containerCreateRequest{
		Image:    "hello-world:latest",
		Hostname: "knot-test",
		HostConfig: containerHostConfig{
			RestartPolicy: restartPolicy{Name: "no"},
		},
	})
	if err != nil {
		t.Fatalf("containerCreate: %v", err)
	}
	if id == "" {
		t.Fatal("containerCreate returned empty ID")
	}
	t.Logf("created container %s", id)

	// Start (hello-world exits immediately — that's fine)
	if err := c.containerStart(ctx, id); err != nil {
		t.Fatalf("containerStart: %v", err)
	}

	// Wait briefly for it to exit
	time.Sleep(500 * time.Millisecond)

	// Inspect
	inspect, code, err := c.containerInspect(ctx, id)
	if err != nil && code != 404 {
		t.Fatalf("containerInspect: %v", err)
	}
	if inspect != nil {
		t.Logf("container state running=%v", inspect.State != nil && inspect.State.Running)
	}

	// Stop (may already be stopped — should not error)
	if err := c.containerStop(ctx, id); err != nil {
		t.Errorf("containerStop: %v", err)
	}

	// Remove
	if err := c.containerRemove(ctx, id); err != nil {
		t.Fatalf("containerRemove: %v", err)
	}

	// Remove again — not-found should be silent
	if err := c.containerRemove(ctx, id); err != nil {
		t.Errorf("containerRemove (not-found) unexpected error: %v", err)
	}
}
