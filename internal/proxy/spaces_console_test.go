package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// TestConsoleReportsFailuresInTheTerminal covers the console's failure UX:
// the websocket is upgraded before the console preconditions run, so a
// failure arrives as a readable [console error] line rather than a popup
// that dies silently. The exact reason depends on the host (this test hits
// the domain-state check, which needs virsh); the assertion is only that a
// reason arrives.
func TestConsoleReportsFailuresInTheTerminal(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	db := database.GetInstance()

	user := model.NewUser("console-tester", "tester@example.com", "password", nil, nil, "", "/bin/sh", "", 1, "", 0, 0, 0)
	if err := db.SaveUser(user, nil); err != nil {
		t.Fatal(err)
	}

	template := model.NewTemplate("kvm-console-test", "", "image: base", "", user.Id, nil,
		model.PlatformKvm, true, false, false, false, false, false, "", "",
		0, 0, false, nil, nil, false, true, 0, "disabled", "", nil)
	if err := db.SaveTemplate(template, nil); err != nil {
		t.Fatal(err)
	}

	space := model.NewSpace("console-space", "", user.Id, template.Id, "/bin/sh", &[]model.AltNameEntry{}, "", "", nil)
	space.ContainerId = "user-console-space"
	if err := db.SaveSpace(space, []string{}); err != nil {
		t.Fatal(err)
	}

	// Route through a mux (the handler reads the space id via PathValue) and
	// inject the authenticated user into the context as ApiAuth would.
	router := http.NewServeMux()
	router.HandleFunc("GET /proxy/spaces/{space_id}/console", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), "user", user))
		HandleSpacesConsoleProxy(w, r)
	})
	server := httptest.NewServer(router)
	defer server.Close()

	url := strings.Replace(server.URL, "http://", "ws://", 1) + "/proxy/spaces/" + space.Id + "/console"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to open console websocket: %v", err)
	}
	defer conn.Close()

	conn.SetReadLimit(4096)
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected an error message before the close, got: %v", err)
	}
	if !strings.Contains(string(message), "[console error]") {
		t.Fatalf("expected a [console error] line, got: %q", string(message))
	}
}

// TestConsoleFallsBackToLocalForPhantomNode covers stale node assignments:
// a space whose node is not a cluster member falls back to the local
// libvirt instead of refusing — here surfacing the domain-state check's
// error (virsh is absent on a dev machine) rather than a forwarding
// failure.
func TestConsoleFallsBackToLocalForPhantomNode(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	db := database.GetInstance()

	user := model.NewUser("console-fallback", "fallback@example.com", "password", nil, nil, "", "/bin/sh", "", 1, "", 0, 0, 0)
	if err := db.SaveUser(user, nil); err != nil {
		t.Fatal(err)
	}

	template := model.NewTemplate("kvm-fallback-test", "", "image: base", "", user.Id, nil,
		model.PlatformKvm, true, false, false, false, false, false, "", "",
		0, 0, false, nil, nil, false, true, 0, "disabled", "", nil)
	if err := db.SaveTemplate(template, nil); err != nil {
		t.Fatal(err)
	}

	space := model.NewSpace("fallback-space", "", user.Id, template.Id, "/bin/sh", &[]model.AltNameEntry{}, "", "", nil)
	space.ContainerId = "user-fallback-space"
	space.NodeId = "00000000-0000-7000-8000-000000000000" // not this node, not in any cluster
	if err := db.SaveSpace(space, []string{}); err != nil {
		t.Fatal(err)
	}

	router := http.NewServeMux()
	router.HandleFunc("GET /proxy/spaces/{space_id}/console", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), "user", user))
		HandleSpacesConsoleProxy(w, r)
	})
	server := httptest.NewServer(router)
	defer server.Close()

	url := strings.Replace(server.URL, "http://", "ws://", 1) + "/proxy/spaces/" + space.Id + "/console"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to open console websocket: %v", err)
	}
	defer conn.Close()

	conn.SetReadLimit(4096)
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected an error message before the close, got: %v", err)
	}
	if !strings.Contains(string(message), "[console error]") {
		t.Fatalf("expected a [console error] line, got: %q", string(message))
	}
	if strings.Contains(string(message), "another node which could not be reached") {
		t.Fatalf("expected the local fallback, not a forwarding refusal: %q", string(message))
	}
}

// TestVNCRequiresTemplateFeature covers the QEMU VNC bridge's gating: a
// space whose template does not enable the display gets a readable refusal
// over the websocket rather than a dead viewer window.
func TestVNCRequiresTemplateFeature(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	db := database.GetInstance()

	// HasPermission resolves against the role cache, which embeds the admin
	// role; seed it the way server boot does.
	model.SetRoleCache(nil)
	user := model.NewUser("vnc-tester", "vnc@example.com", "password", []string{model.RoleAdminUUID}, nil, "", "/bin/sh", "", 1, "", 0, 0, 0)
	if err := db.SaveUser(user, nil); err != nil {
		t.Fatal(err)
	}

	// KVM template with the display disabled (the default).
	template := model.NewTemplate("kvm-vnc-off", "", "image: base", "", user.Id, nil,
		model.PlatformKvm, true, false, false, false, false, false, "", "",
		0, 0, false, nil, nil, false, true, 0, "disabled", "", nil)
	if err := db.SaveTemplate(template, nil); err != nil {
		t.Fatal(err)
	}

	space := model.NewSpace("vnc-space", "", user.Id, template.Id, "/bin/sh", &[]model.AltNameEntry{}, "", "", nil)
	space.ContainerId = "user-vnc-space"
	if err := db.SaveSpace(space, []string{}); err != nil {
		t.Fatal(err)
	}

	router := http.NewServeMux()
	router.HandleFunc("GET /proxy/spaces/{space_id}/vnc", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), "user", user))
		HandleSpacesVNCProxy(w, r)
	})
	server := httptest.NewServer(router)
	defer server.Close()

	url := strings.Replace(server.URL, "http://", "ws://", 1) + "/proxy/spaces/" + space.Id + "/vnc"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to open vnc websocket: %v", err)
	}
	defer conn.Close()

	conn.SetReadLimit(4096)
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected an error message before the close, got: %v", err)
	}
	if !strings.Contains(string(message), "does not expose the VM's display") {
		t.Fatalf("expected a template-feature refusal, got: %q", string(message))
	}
}

// TestVNCBridgeRelaysRFB proves the websocket↔TCP bridge relays bytes both
// ways: a fake virsh (domstate/domdisplay) plus a fake RFB server standing
// in for QEMU's VNC. If this passes, a stuck "connecting" viewer is an
// environment issue (display missing, old binary on the owning node), not
// the bridge.
func TestVNCBridgeRelaysRFB(t *testing.T) {
	// Fake virsh answering the two probes the handler makes; the display
	// number (10) maps to TCP port 5910 where the fake RFB server listens.
	binDir := t.TempDir()
	virsh := "#!/bin/sh\ncase \"$3\" in domstate) echo running ;; domdisplay) echo 'vnc://127.0.0.1:10' ;; *) exit 1 ;; esac\n"
	if err := os.WriteFile(filepath.Join(binDir, "virsh"), []byte(virsh), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	// Fake RFB server: QEMU's VNC speaks first with the version banner, then
	// echoes anything it receives back. Bind inside the real VNC port range
	// (5900 + display number, display <= 100) so the derived address is one
	// the parser accepts.
	var listener net.Listener
	display := -1
	for d := 90; d >= 0 && display < 0; d-- {
		if l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", 5900+d)); err == nil {
			listener = l
			display = d
		}
	}
	if listener == nil {
		t.Skip("no free port in the VNC range 5900-5990")
	}
	defer listener.Close()
	virsh = fmt.Sprintf("#!/bin/sh\ncase \"$3\" in domstate) echo running ;; domdisplay) echo 'vnc://127.0.0.1:%d' ;; *) exit 1 ;; esac\n", display)
	if err := os.WriteFile(filepath.Join(binDir, "virsh"), []byte(virsh), 0755); err != nil {
		t.Fatal(err)
	}

	rfbDone := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			rfbDone <- ""
			return
		}
		defer conn.Close()
		conn.Write([]byte("RFB 003.008\n"))
		buf := make([]byte, 64)
		n, _ := conn.Read(buf)
		rfbDone <- string(buf[:n])
	}()

	// Space setup: KVM template with the display enabled, running space.
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	db := database.GetInstance()
	model.SetRoleCache(nil)
	user := model.NewUser("vnc-relay", "relay@example.com", "password", []string{model.RoleAdminUUID}, nil, "", "/bin/sh", "", 1, "", 0, 0, 0)
	if err := db.SaveUser(user, nil); err != nil {
		t.Fatal(err)
	}

	template := model.NewTemplate("kvm-vnc-relay", "", "image: base", "", user.Id, nil,
		model.PlatformKvm, true, false, false, false, false, false, "", "",
		0, 0, false, nil, nil, false, true, 0, "disabled", "", nil)
	template.WithVNC = true
	if err := db.SaveTemplate(template, nil); err != nil {
		t.Fatal(err)
	}

	space := model.NewSpace("relay-space", "", user.Id, template.Id, "/bin/sh", &[]model.AltNameEntry{}, "", "", nil)
	space.ContainerId = "user-relay-space"
	if err := db.SaveSpace(space, []string{}); err != nil {
		t.Fatal(err)
	}

	router := http.NewServeMux()
	router.HandleFunc("GET /proxy/spaces/{space_id}/vnc", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), "user", user))
		HandleSpacesVNCProxy(w, r)
	})
	server := httptest.NewServer(router)
	defer server.Close()

	url := strings.Replace(server.URL, "http://", "ws://", 1) + "/proxy/spaces/" + space.Id + "/vnc"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to open vnc websocket: %v", err)
	}
	defer conn.Close()

	// The server-first RFB banner must arrive over the websocket.
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, banner, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected the RFB banner over the websocket: %v", err)
	}
	if string(banner) != "RFB 003.008\n" {
		t.Fatalf("unexpected banner: %q", string(banner))
	}

	// And client input must reach the TCP side.
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("client-hello")); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-rfbDone:
		if got != "client-hello" {
			t.Fatalf("fake RFB server received %q", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for client input to reach the RFB server")
	}
}
