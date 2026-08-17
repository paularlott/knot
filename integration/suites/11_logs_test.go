//go:build integration

package suites

import (
	"testing"
	"time"

	"github.com/paularlott/knot/integration/harness"
)

func TestSyslogToLogStream(t *testing.T) {
	harness.Feature(t, "logs")
	id := workspace(t)

	// Connect the websocket log stream first so no lines are missed. Reads
	// run in a goroutine with a recover guard: gorilla panics ("repeated
	// read on failed websocket connection") when a read deadline expires
	// mid-frame and permanently fails the connection.
	wsURL := user1.Client.GetWebSocketURL() + "/logs/" + id + "/stream"
	ctx, cancel := testCtx(120)
	defer cancel()
	ws, _, err := wsDial(ctx, user1.Token, wsURL)
	if err != nil {
		t.Fatalf("dial log stream: %v", err)
	}
	defer ws.Close()

	messages := make(chan string, 64)
	go func() {
		defer func() { recover() }()
		for {
			ws.SetReadDeadline(time.Now().Add(10 * time.Second))
			_, data, err := ws.ReadMessage()
			if err != nil {
				close(messages)
				return
			}
			select {
			case messages <- string(data):
			default:
			}
		}
	}()

	// The boot history (rsyslog startup lines) should arrive immediately;
	// wait for the stream to be live, then emit the marker twice: once
	// through the container's rsyslog (`logger`) and once directly to the
	// agent's syslogd UDP port, bypassing rsyslog config entirely.
	marker := "it-syslog-marker-" + uniqueName("x")
	live := waitForChan(t, messages, 20*time.Second, func(m string) bool {
		return contains(m, "rsyslogd") || contains(m, "System started")
	})
	if !live {
		t.Fatal("log stream did not deliver boot history")
	}

	harness.TryRunCommand(user1.Client, id, 30, "logger "+marker)
	harness.TryRunCommand(user1.Client, id, 30,
		"echo '<13>"+marker+" via-udp' > /dev/udp/127.0.0.1/1514")

	deadline := time.After(60 * time.Second)
	for {
		select {
		case m, ok := <-messages:
			if !ok {
				t.Fatal("log stream closed before the marker arrived")
			}
			if contains(m, marker) {
				return
			}
		case <-deadline:
			t.Fatalf("marker %q never appeared in the log stream", marker)
		}
	}
}

// waitForChan consumes messages until cond matches or the timeout elapses.
func waitForChan(t *testing.T, ch <-chan string, timeout time.Duration, cond func(string) bool) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case m, ok := <-ch:
			if !ok {
				return false
			}
			if cond(m) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}
