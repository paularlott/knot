package agent_client

import (
	"fmt"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

// TestLogsBeforeConnectionAreKept proves log lines a space writes before the
// agent is connected to the server, such as its boot logs while the agent
// registers, are held and delivered in order once the connection is up,
// rather than dropped.
func TestLogsBeforeConnectionAreKept(t *testing.T) {
	c := NewAgentClient("test.example.com:443", "space-1")
	s := NewAgentServer("test.example.com:443", "space-1", c)
	t.Cleanup(s.cancel)

	// No connection yet: these must wait, not vanish.
	for i := range 5 {
		s.logChannel <- &msg.LogMessage{Message: fmt.Sprintf("early-%d", i)}
	}
	time.Sleep(2 * logRetryInterval)

	// The connection comes up.
	muxClient, muxServer := newTestMuxPair(t)
	c.serverListMutex.Lock()
	s.muxSession = muxClient
	c.serverListMutex.Unlock()
	s.logChannel <- &msg.LogMessage{Message: "late"}

	stream, err := muxServer.Accept()
	if err != nil {
		t.Fatalf("accept log stream: %v", err)
	}
	defer stream.Close()

	want := []string{"early-0", "early-1", "early-2", "early-3", "early-4", "late"}
	for _, w := range want {
		stream.SetReadDeadline(time.Now().Add(5 * time.Second))
		cmd, err := msg.ReadCommand(stream)
		if err != nil || cmd != byte(msg.CmdLogMessage) {
			t.Fatalf("waiting for %q: command %d, %v", w, cmd, err)
		}
		var got msg.LogMessage
		if err := msg.ReadMessage(stream, &got); err != nil {
			t.Fatalf("waiting for %q: %v", w, err)
		}
		if got.Message != w {
			t.Fatalf("got %q, want %q", got.Message, w)
		}
	}
}

// TestLogBacklogIsBounded proves a long time without a connection keeps
// only the newest lines, so memory stays bounded.
func TestLogBacklogIsBounded(t *testing.T) {
	c := NewAgentClient("test.example.com:443", "space-1")
	s := NewAgentServer("test.example.com:443", "space-1", c)
	t.Cleanup(s.cancel)

	total := logBacklogLimit + 10
	for i := range total {
		s.logChannel <- &msg.LogMessage{Message: fmt.Sprintf("m-%d", i)}
	}
	// Let the worker take them all off the queue.
	deadline := time.Now().Add(5 * time.Second)
	for len(s.logChannel) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(2 * logRetryInterval)

	muxClient, muxServer := newTestMuxPair(t)
	c.serverListMutex.Lock()
	s.muxSession = muxClient
	c.serverListMutex.Unlock()

	stream, err := muxServer.Accept()
	if err != nil {
		t.Fatalf("accept log stream: %v", err)
	}
	defer stream.Close()

	for i := total - logBacklogLimit; i < total; i++ {
		stream.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := msg.ReadCommand(stream); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		var got msg.LogMessage
		if err := msg.ReadMessage(stream, &got); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		if want := fmt.Sprintf("m-%d", i); got.Message != want {
			t.Fatalf("got %q, want %q: the oldest lines should go first", got.Message, want)
		}
	}
}
