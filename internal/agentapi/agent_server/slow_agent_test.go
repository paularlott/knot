package agent_server

import (
	"net"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/log"
)

// slowAgentSession is a session to a fake agent that answers every request
// after delay with reply.
func slowAgentSession(t *testing.T, delay time.Duration, reply any) *Session {
	t.Helper()
	c1, c2 := net.Pipe()
	agent, err := yamux.Server(c2, nil)
	if err != nil {
		t.Fatal(err)
	}
	server, err := yamux.Client(c1, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		server.Close()
		agent.Close()
	})
	go func() {
		for {
			stream, err := agent.Accept()
			if err != nil {
				return
			}
			go func() {
				defer stream.Close()
				if _, err := msg.ReadCommand(stream); err != nil {
					return
				}
				var req map[string]any
				msg.ReadMessage(stream, &req) // a request with no message times out harmlessly
				time.Sleep(delay)
				msg.WriteMessage(stream, reply)
			}()
		}
	}()
	return &Session{Id: "space-slow", MuxSession: server, logger: log.WithGroup("test")}
}

// An agent may take longer than a message's default five seconds over work:
// copying a large file, or starting a port forward on a busy host.
func TestSlowAgentAnswersAreWaitedFor(t *testing.T) {
	t.Run("copy a file", func(t *testing.T) {
		t.Parallel()
		s := slowAgentSession(t, 6*time.Second, &msg.CopyFileResponse{Success: true})
		ch, err := s.SendCopyFile(&msg.CopyFileMessage{})
		if err != nil {
			t.Fatal(err)
		}
		if resp := <-ch; resp == nil || !resp.Success {
			t.Fatalf("a copy answered after 6 seconds failed: %+v", resp)
		}
	})
	t.Run("list port forwards", func(t *testing.T) {
		t.Parallel()
		s := slowAgentSession(t, 6*time.Second, &msg.PortListResponse{})
		if _, err := s.SendPortList(); err != nil {
			t.Fatalf("a port list answered after 6 seconds failed: %v", err)
		}
	})
	t.Run("run a command", func(t *testing.T) {
		t.Parallel()
		s := slowAgentSession(t, 6*time.Second, &msg.RunCommandResponse{Success: true})
		ch, err := s.SendRunCommand(&msg.RunCommandMessage{Command: "true", Timeout: 10})
		if err != nil {
			t.Fatal(err)
		}
		if resp := <-ch; resp == nil || !resp.Success {
			t.Fatalf("a command answered after 6 seconds failed: %+v", resp)
		}
	})
}
