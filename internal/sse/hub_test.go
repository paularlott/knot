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

// An event for one user reaches only that user's streams.
func TestSendToUser(t *testing.T) {
	h := &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan *Event, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		shutdown:   make(chan struct{}),
	}
	go h.run()
	defer close(h.shutdown)

	a1 := h.NewClient("alice", "s1")
	a2 := h.NewClient("alice", "s2")
	b := h.NewClient("bob", "s3")
	if ids := h.UserIds(); len(ids) != 2 {
		t.Fatalf("user ids %v", ids)
	}

	h.SendToUser("alice", &Event{Type: EventFilesChanged, Payload: FilesPayload{BucketIds: []string{"x"}}})
	for _, c := range []*Client{a1, a2} {
		select {
		case data := <-c.Send():
			if string(data) != `{"type":"files:changed","payload":{"bucket_ids":["x"]}}` {
				t.Errorf("got %s", data)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("alice's stream got nothing")
		}
	}
	select {
	case data := <-b.Send():
		t.Errorf("bob got %s", data)
	case <-time.After(100 * time.Millisecond):
	}

	// Broadcasts still reach everyone.
	h.Broadcast(&Event{Type: EventTemplatesChanged})
	for _, c := range []*Client{a1, a2, b} {
		select {
		case <-c.Send():
		case <-time.After(2 * time.Second):
			t.Fatal("broadcast missed a stream")
		}
	}
}
