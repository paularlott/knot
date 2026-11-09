package agent_client

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/methods"
)

// fakeKnotServer answers the agent's method registration commands over a
// yamux session: it refuses registrations containing a method named "bad"
// and counts unregister commands.
type fakeKnotServer struct {
	mu          sync.Mutex
	registered  []string // method names of the last accepted registration
	unregisters int
}

func (f *fakeKnotServer) serve(session *yamux.Session) {
	for {
		stream, err := session.Accept()
		if err != nil {
			return
		}
		go f.handle(stream)
	}
}

func (f *fakeKnotServer) handle(stream net.Conn) {
	defer stream.Close()
	cmd, err := msg.ReadCommand(stream)
	if err != nil {
		return
	}
	switch msg.CmdType(cmd) {
	case msg.CmdRegisterMethods:
		var req msg.RegisterMethodsRequest
		if err := msg.ReadMessage(stream, &req); err != nil {
			return
		}
		var names []string
		for _, m := range req.Registration.Methods {
			if m.Name == "bad" {
				_ = msg.WriteMessage(stream, &msg.RegisterMethodsResponse{Error: "refused"})
				return
			}
			names = append(names, m.Name)
		}
		f.mu.Lock()
		f.registered = names
		f.mu.Unlock()
		_ = msg.WriteMessage(stream, &msg.RegisterMethodsResponse{Success: true})
	case msg.CmdUnregisterMethods:
		f.mu.Lock()
		f.unregisters++
		f.registered = nil
		f.mu.Unlock()
	}
}

func (f *fakeKnotServer) state() ([]string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.registered, f.unregisters
}

// newRegisterTestClient returns an agent client connected to a fake knot
// server.
func newRegisterTestClient(t *testing.T) (*AgentClient, *fakeKnotServer) {
	t.Helper()
	c1, c2 := net.Pipe()
	serverSession, err := yamux.Server(c2, nil)
	if err != nil {
		t.Fatalf("yamux.Server: %v", err)
	}
	clientSession, err := yamux.Client(c1, nil)
	if err != nil {
		t.Fatalf("yamux.Client: %v", err)
	}
	fake := &fakeKnotServer{}
	go fake.serve(serverSession)

	c := NewAgentClient("default.example.com:443", "space-1")
	s := NewAgentServer("default.example.com:443", "space-1", c)
	c.serverListMutex.Lock()
	c.serverList[s.address] = s
	c.serverListMutex.Unlock()
	s.setMux(clientSession)

	t.Cleanup(func() {
		c.stopMethodServer()
		serverSession.Close()
		clientSession.Close()
		c1.Close()
		c2.Close()
	})
	return c, fake
}

func catRegistration(names ...string) *methods.Registration {
	reg := &methods.Registration{Server: methods.ServerConfig{Type: "stdio", Command: "cat"}}
	for _, name := range names {
		reg.Methods = append(reg.Methods, methods.MethodDefinition{Name: name, Description: name})
	}
	return reg
}

func TestRefusedRegistrationKeepsRunningMethods(t *testing.T) {
	c, fake := newRegisterTestClient(t)

	good := catRegistration("good")
	if err := c.RegisterMethods(good); err != nil {
		t.Fatalf("register good: %v", err)
	}
	c.methodMu.RLock()
	running := c.methodServer
	c.methodMu.RUnlock()

	if err := c.RegisterMethods(catRegistration("bad")); err == nil {
		t.Fatal("refused registration returned no error")
	}

	// Give the refused server's process time to exit.
	time.Sleep(200 * time.Millisecond)

	c.methodMu.RLock()
	current := c.methodServer
	c.methodMu.RUnlock()
	if current != running {
		t.Fatal("refused registration replaced the running method server")
	}
	select {
	case <-running.closed:
		t.Fatal("refused registration stopped the running method server")
	default:
	}
	c.lastRegMu.RLock()
	lastReg := c.lastReg
	c.lastRegMu.RUnlock()
	if lastReg != good {
		t.Fatal("refused registration replaced the stashed registration")
	}
	if registered, unregisters := fake.state(); unregisters != 0 || len(registered) != 1 || registered[0] != "good" {
		t.Fatalf("knot server has %v after %d unregisters, want [good] and none", registered, unregisters)
	}
}

func TestReplacedServerExitKeepsNewMethods(t *testing.T) {
	c, fake := newRegisterTestClient(t)

	if err := c.RegisterMethods(catRegistration("first")); err != nil {
		t.Fatalf("register first: %v", err)
	}
	c.methodMu.RLock()
	first := c.methodServer
	c.methodMu.RUnlock()

	if err := c.RegisterMethods(catRegistration("second")); err != nil {
		t.Fatalf("register second: %v", err)
	}

	select {
	case <-first.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("replaced method server was not stopped")
	}
	// Give the replaced process's exit hook time to run.
	time.Sleep(200 * time.Millisecond)

	if registered, unregisters := fake.state(); unregisters != 0 || len(registered) != 1 || registered[0] != "second" {
		t.Fatalf("knot server has %v after %d unregisters, want [second] and none", registered, unregisters)
	}
	c.lastRegMu.RLock()
	lastReg := c.lastReg
	c.lastRegMu.RUnlock()
	if lastReg == nil || lastReg.Methods[0].Name != "second" {
		t.Fatal("replaced server's exit cleared the new registration")
	}
}

func TestRunningServerExitUnregistersMethods(t *testing.T) {
	c, fake := newRegisterTestClient(t)

	if err := c.RegisterMethods(catRegistration("only")); err != nil {
		t.Fatalf("register: %v", err)
	}
	c.methodMu.RLock()
	running := c.methodServer
	c.methodMu.RUnlock()

	// The process exits while it is still the running server, as on a crash.
	running.shutdown()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, unregisters := fake.state(); unregisters == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("running server's exit did not unregister its methods")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
