package sse

import (
	"testing"
	"time"
)

// TestCloseSessionDropsOnlyThatSession pins the fast-user-switch contract:
// CloseSession ends the target session's streams WITHOUT the
// auth:required event — the client answers that by navigating to /logout,
// which deletes the just-switched session — and leaves other sessions'
// streams untouched.
func TestCloseSessionDropsOnlyThatSession(t *testing.T) {
	hub := GetHub()
	hub.Start()
	t.Cleanup(hub.Shutdown)

	a := hub.NewClient("user-a", "session-a")
	b := hub.NewClient("user-b", "session-b")
	defer a.Close()
	defer b.Close()

	// Registration is an unbuffered handoff into the run loop; wait for
	// both clients to land in the map before closing.
	deadline := time.Now().Add(2 * time.Second)
	hub.mu.Lock()
	for len(hub.clients) < 2 && time.Now().Before(deadline) {
		hub.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		hub.mu.Lock()
	}
	if len(hub.clients) < 2 {
		hub.mu.Unlock()
		t.Fatal("clients did not register")
	}
	hub.mu.Unlock()

	hub.CloseSession("session-a")

	// a's stream ends with no event delivered before the close.
	select {
	case data, ok := <-a.Send():
		if ok {
			t.Fatalf("client a received %q; CloseSession must not deliver events", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client a's send channel was not closed")
	}

	// b's stream stays open and quiet.
	select {
	case data, ok := <-b.Send():
		if ok {
			t.Fatalf("client b received %q; unexpected event", data)
		}
		t.Fatal("client b's send channel closed; CloseSession must not touch other sessions")
	case <-time.After(100 * time.Millisecond):
	}
}
