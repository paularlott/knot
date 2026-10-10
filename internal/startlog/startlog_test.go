package startlog

import (
	"testing"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

func TestBeginResetsAndSubscribeStreams(t *testing.T) {
	const id = "space-1"
	Begin(id, "first %d", 1)
	Info(id, "step")
	Begin(id, "second")

	history, lines, cancel := Subscribe(id)
	defer cancel()
	if len(history) != 1 || history[0].Message != "second" {
		t.Fatalf("history after Begin = %+v, want only the new start", history)
	}

	Error(id, "boom: %s", "pull failed")
	select {
	case m := <-lines:
		if m.Level != msg.LogLevelError || m.Message != "boom: pull failed" || m.Service != Service {
			t.Fatalf("streamed line = %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("no line streamed")
	}

	cancel()
	cancel() // idempotent
	if _, ok := <-lines; ok {
		t.Fatal("channel not closed by cancel")
	}
}

func TestEntriesAreCapped(t *testing.T) {
	const id = "space-cap"
	Begin(id, "start")
	for i := 0; i < maxEntries+50; i++ {
		Info(id, "line %d", i)
	}
	history, _, cancel := Subscribe(id)
	defer cancel()
	if len(history) != maxEntries {
		t.Fatalf("len(history) = %d, want %d", len(history), maxEntries)
	}
}
