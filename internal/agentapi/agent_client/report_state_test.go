package agent_client

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/paularlott/knot/internal/agentapi/msg"
)

func newTestMuxPair(t *testing.T) (*yamux.Session, *yamux.Session) {
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
	return client, server
}

// TestConcurrentReportEvent fires many ReportEvent calls simultaneously
// against a single mux session. Before the fix (shared reportingConn), this
// corrupted the wire protocol and caused intermittent "no servers accepted"
// errors. With per-event fresh streams, all calls must succeed.
func TestConcurrentReportEvent(t *testing.T) {
	c := NewAgentClient("test.example.com:443", "space-1")

	muxClient, muxServer := newTestMuxPair(t)

	s := NewAgentServer("test.example.com:443", "space-1", c)
	s.muxSession = muxClient

	c.serverListMutex.Lock()
	c.serverList[s.address] = s
	c.serverListMutex.Unlock()

	var received int64

	// Server side: accept streams, read CmdEvent, write reply.
	var serverWG sync.WaitGroup
	serverDone := make(chan struct{})
	go func() {
		for {
			stream, err := muxServer.Accept()
			if err != nil {
				close(serverDone)
				return
			}
			serverWG.Add(1)
			go func(conn net.Conn) {
				defer conn.Close()
				defer serverWG.Done()

				cmd, err := msg.ReadCommand(conn)
				if err != nil {
					return
				}

				switch cmd {
				case byte(msg.CmdEvent):
					var eventMsg msg.Event
					if err := msg.ReadMessage(conn, &eventMsg); err != nil {
						return
					}
					atomic.AddInt64(&received, 1)
					_ = msg.WriteMessage(conn, &msg.EventReply{})
				}
			}(stream)
		}
	}()

	const numEvents = 100
	var clientWG sync.WaitGroup
	clientWG.Add(numEvents)

	for i := 0; i < numEvents; i++ {
		go func(n int) {
			defer clientWG.Done()
			event := &msg.Event{
				EventId:   fmt.Sprintf("evt-%d", n),
				EventType: "test.event",
				SpaceId:   "space-1",
			}
			if err := c.ReportEvent(event); err != nil {
				t.Errorf("ReportEvent %d failed: %v", n, err)
			}
		}(i)
	}

	clientWG.Wait()
	muxServer.Close()
	<-serverDone
	serverWG.Wait()

	got := atomic.LoadInt64(&received)
	if got != numEvents {
		t.Errorf("expected %d events received, got %d", numEvents, got)
	}
}
