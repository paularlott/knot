package nfs

import (
	"testing"
	"time"
)

// knot: a rename or remove waits for the requests read before it on its
// connection, not for those read after.
func TestWaitEarlier(t *testing.T) {
	c := &conn{}
	first := c.startRequest()
	second := c.startRequest()
	third := c.startRequest()
	w := &response{conn: c, seq: second}

	done := make(chan struct{})
	go func() { w.waitEarlier(); close(done) }()
	select {
	case <-done:
		t.Fatal("did not wait for the request read before it")
	case <-time.After(50 * time.Millisecond):
	}
	c.endRequest(first)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("still waiting once the earlier request was handled")
	}
	_ = third // read after: not waited for

	// Not handled concurrently (no number): no wait.
	(&response{conn: &conn{}}).waitEarlier()
	var nilW *response
	nilW.waitEarlier()
}
