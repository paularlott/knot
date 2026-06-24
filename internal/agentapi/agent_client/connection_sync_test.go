package agent_client

import (
	"net"
	"sync"
	"testing"

	"github.com/hashicorp/yamux"
)

// newTestMux returns a live yamux client session backed by an in-memory pipe.
// It must be called from the test goroutine. The session and its pipe are
// closed when the test finishes.
func newTestMux(t *testing.T) *yamux.Session {
	t.Helper()
	c1, c2 := net.Pipe()
	server, err := yamux.Server(c2, nil)
	if err != nil {
		t.Fatalf("yamux.Server: %v", err)
	}
	client, err := yamux.Client(c1, nil)
	if err != nil {
		t.Fatalf("yamux.Client: %v", err)
	}
	t.Cleanup(func() {
		server.Close()
		client.Close()
		c1.Close()
		c2.Close()
	})
	return client
}

// TestConnectionFieldsAreSynchronizedAcrossReconnect reproduces the production
// access pattern that left agents stuck after a server restart: the connection
// goroutine (ConnectAndServe) swaps s.muxSession on every reconnect, while the
// reportState / logWorker / method-server goroutines read s.muxSession under
// serverListMutex. When the swap happened without taking the write lock there
// was a data race (and, worse, a reader could keep observing the previous,
// closed session forever — "mux ping succeeds but agent state is stale").
//
// The mutation now goes through setMux / teardownConnections, which take the
// write lock the readers already hold for reading. Run with -race; any
// regression that writes the connection fields without the lock fails here.
func TestConnectionFieldsAreSynchronizedAcrossReconnect(t *testing.T) {
	c := NewAgentClient("default.example.com:443", "space-1")
	s := NewAgentServer("default.example.com:443", "space-1", c)

	c.serverListMutex.Lock()
	c.serverList[s.address] = s
	c.serverListMutex.Unlock()

	// Pre-build the sessions on the test goroutine (t.Fatalf/t.Cleanup must not
	// be called from other goroutines).
	const reconnects = 150
	sessions := make([]*yamux.Session, reconnects)
	for i := range sessions {
		sessions[i] = newTestMux(t)
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup

	// Readers mirror reportState's guard: read s.muxSession under RLock.
	reader := func() {
		defer readers.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			c.serverListMutex.RLock()
			for _, srv := range c.serverList {
				if srv.muxSession != nil && !srv.muxSession.IsClosed() {
					_ = srv.muxSession
				}
			}
			c.serverListMutex.RUnlock()
		}
	}

	for i := 0; i < 3; i++ {
		readers.Add(1)
		go reader()
	}

	// The connection goroutine repeatedly establishes and tears down a mux
	// session through the locked helpers, simulating reconnect churn.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for _, sess := range sessions {
			s.setConn(nil)
			s.setMux(sess)
			s.teardownConnections()
		}
	}()

	<-writerDone
	close(stop)
	readers.Wait()

	// After the final teardown the session must be cleared.
	c.serverListMutex.RLock()
	defer c.serverListMutex.RUnlock()
	if s.muxSession != nil {
		t.Fatal("muxSession should be nil after teardownConnections")
	}
}
