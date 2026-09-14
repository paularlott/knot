package tunnel_server

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/paularlott/knot/internal/wsconn"
)

// fakeTunnelServer stands in for a knot server's tunnel endpoints: the
// /api/users/whoami and /api/tunnels/server-info calls TunnelClient makes
// before connecting, and the /tunnel/server/{name} websocket the tunnel
// itself runs over. The listener can be closed and rebound on the same port
// to simulate a server restart.
type fakeTunnelServer struct {
	url      string
	upgrades atomic.Int32

	mu      sync.Mutex
	conns   []*websocket.Conn // every websocket ever accepted, so a restart can drop them
	live    atomic.Pointer[yamux.Session]
	closed  atomic.Int32 // sessions whose connection the client side closed
}

func (f *fakeTunnelServer) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/users/whoami", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"username": "tester"})
	})
	mux.HandleFunc("/api/tunnels/server-info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"domain": ".tunnels.example.com", "tunnel_servers": []string{f.url}})
	})
	mux.HandleFunc("/tunnel/server/", func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}

		f.mu.Lock()
		f.conns = append(f.conns, ws)
		f.mu.Unlock()
		f.upgrades.Add(1)

		session, err := yamux.Server(wsconn.New(ws), nil)
		if err != nil {
			ws.Close()
			return
		}
		f.live.Store(session)
		go func() {
			<-session.CloseChan()
			f.closed.Add(1)
		}()
	})

	return mux
}

// dropConnections closes every websocket the server accepted, as a crash
// would. (http.Server.Close cannot reach them: they are hijacked.)
func (f *fakeTunnelServer) dropConnections() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ws := range f.conns {
		ws.Close()
	}
	f.conns = nil
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(body)
}

func serveTunnelServer(t *testing.T, f *fakeTunnelServer, l net.Listener) *http.Server {
	t.Helper()
	server := &http.Server{Handler: f.handler()}
	go server.Serve(l)
	return server
}

func echoListener(t *testing.T) net.Listener {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start echo listener: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				io.Copy(conn, conn)
			}()
		}
	}()

	return listener
}

// proxiedRoundTrip opens a stream from the server side of the tunnel (as the
// real server does to carry an incoming request), writes the non-zero
// connection marker and a payload, and expects the payload echoed back from
// the port the client forwards to.
func proxiedRoundTrip(t *testing.T, session *yamux.Session, payload string) {
	t.Helper()

	stream, err := session.Open()
	if err != nil {
		t.Fatalf("failed to open stream on tunnel session: %v", err)
	}
	defer stream.Close()
	stream.SetDeadline(time.Now().Add(10 * time.Second))

	if _, err = stream.Write([]byte{1}); err != nil {
		t.Fatalf("failed to write connection marker: %v", err)
	}
	if _, err = fmt.Fprintf(stream, "%s", payload); err != nil {
		t.Fatalf("failed to write payload: %v", err)
	}

	buf := make([]byte, len(payload))
	if _, err = io.ReadFull(stream, buf); err != nil {
		t.Fatalf("failed to read echoed payload: %v", err)
	}
	if string(buf) != payload {
		t.Fatalf("unexpected echo: got %q, want %q", string(buf), payload)
	}
}

// waitFor polls until cond passes or the timeout expires.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestWebTunnelReconnectsAfterServerRestart covers the daemon-mode
// requirement: a web tunnel must survive a knot server restart. The outage
// here (6s) is longer than the tunnel's old 5-attempt reconnect budget, so a
// client that gave up would fail this test.
func TestWebTunnelReconnectsAfterServerRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping reconnect timing test in short mode")
	}

	echo := echoListener(t)
	defer echo.Close()
	echoPort := echo.Addr().(*net.TCPAddr).Port

	fake := &fakeTunnelServer{}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start tunnel server listener: %v", err)
	}
	fake.url = "http://" + listener.Addr().String()
	server := serveTunnelServer(t, fake, listener)

	client := NewTunnelClient("ws"+fake.url[4:], fake.url, "token", true, &TunnelOpts{
		Type:      WebTunnel,
		Protocol:  "http",
		LocalPort: uint16(echoPort),
		TunnelName: "reconnect",
	})
	if err = client.ConnectAndServe(); err != nil {
		t.Fatalf("failed to start tunnel client: %v", err)
	}
	defer client.Shutdown()

	if url := client.URL(); url != "https://tester--reconnect.tunnels.example.com" {
		t.Fatalf("unexpected tunnel URL: %s", url)
	}

	// First connection is live and proxies traffic. ConnectAndServe returns
	// before the websocket is established, so wait for it.
	waitFor(t, 10*time.Second, "initial tunnel session", func() bool {
		return fake.live.Load() != nil
	})
	firstSession := fake.live.Load()
	proxiedRoundTrip(t, firstSession, "before-restart")

	// Simulate a server crash: listener closed, connections dropped.
	listener.Close()
	server.Close()
	fake.dropConnections()

	// Stay down for longer than the tunnel's old reconnect budget, proving
	// the client does not give up while the server is away.
	time.Sleep(6 * time.Second)

	listener, err = net.Listen("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to rebind tunnel server listener: %v", err)
	}
	server = serveTunnelServer(t, fake, listener)
	defer server.Close()
	defer listener.Close()

	// The client reconnects on its own and the reformed tunnel carries
	// traffic again.
	waitFor(t, 30*time.Second, "tunnel to reconnect after server restart", func() bool {
		return fake.upgrades.Load() >= 2 && fake.live.Load() != firstSession
	})
	proxiedRoundTrip(t, fake.live.Load(), "after-restart")
}

// TestShutdownClosesTunnelAndStaysDead covers `knot tunnel stop`: Shutdown
// must close the live websocket (so the server drops the tunnel) and the
// client must not reconnect afterwards, even though reconnecting is now
// unbounded.
func TestShutdownClosesTunnelAndStaysDead(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping shutdown timing test in short mode")
	}

	echo := echoListener(t)
	defer echo.Close()
	echoPort := echo.Addr().(*net.TCPAddr).Port

	fake := &fakeTunnelServer{}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start tunnel server listener: %v", err)
	}
	defer listener.Close()
	fake.url = "http://" + listener.Addr().String()
	server := serveTunnelServer(t, fake, listener)
	defer server.Close()

	client := NewTunnelClient("ws"+fake.url[4:], fake.url, "token", true, &TunnelOpts{
		Type:       WebTunnel,
		Protocol:   "http",
		LocalPort:  uint16(echoPort),
		TunnelName: "stop-me",
	})
	if err = client.ConnectAndServe(); err != nil {
		t.Fatalf("failed to start tunnel client: %v", err)
	}

	waitFor(t, 10*time.Second, "initial tunnel session", func() bool {
		return fake.live.Load() != nil
	})

	client.Shutdown()

	// The server sees the connection close.
	waitFor(t, 10*time.Second, "server to see the tunnel close", func() bool {
		return fake.closed.Load() >= 1
	})

	// And the client stays gone: no reconnect attempts.
	time.Sleep(4 * time.Second)
	if n := fake.upgrades.Load(); n != 1 {
		t.Fatalf("tunnel reconnected after Shutdown: %d websocket upgrades", n)
	}
}

// TestServerCloseRequestStopsTunnel covers the server asking a tunnel to
// close (the byte-0 stream the server sends when a tunnel is deleted): the
// client must shut down and must NOT reconnect, even though reconnecting is
// now unbounded.
func TestServerCloseRequestStopsTunnel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping close-request timing test in short mode")
	}

	echo := echoListener(t)
	defer echo.Close()
	echoPort := echo.Addr().(*net.TCPAddr).Port

	fake := &fakeTunnelServer{}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start tunnel server listener: %v", err)
	}
	defer listener.Close()
	fake.url = "http://" + listener.Addr().String()
	server := serveTunnelServer(t, fake, listener)
	defer server.Close()

	client := NewTunnelClient("ws"+fake.url[4:], fake.url, "token", true, &TunnelOpts{
		Type:       WebTunnel,
		Protocol:   "http",
		LocalPort:  uint16(echoPort),
		TunnelName: "close-me",
	})
	if err = client.ConnectAndServe(); err != nil {
		t.Fatalf("failed to start tunnel client: %v", err)
	}
	defer client.Shutdown()

	waitFor(t, 10*time.Second, "initial tunnel session", func() bool {
		return fake.live.Load() != nil
	})

	// The server deletes the tunnel: close-request marker, then the session.
	session := fake.live.Load()
	stream, err := session.Open()
	if err != nil {
		t.Fatalf("failed to open stream: %v", err)
	}
	stream.Write([]byte{0})
	stream.Close()
	session.Close()

	// The client context dies (the registry entry goes with it).
	waitFor(t, 10*time.Second, "client context to be cancelled", func() bool {
		select {
		case <-client.GetCtx().Done():
			return true
		default:
			return false
		}
	})

	// And the client stays gone: no reconnect attempts.
	time.Sleep(4 * time.Second)
	if n := fake.upgrades.Load(); n != 1 {
		t.Fatalf("tunnel reconnected after server close request: %d websocket upgrades", n)
	}
}
