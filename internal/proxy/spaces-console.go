package proxy

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container/kvm"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/validate"
)

// writeConsoleError reports a failure into the terminal before closing: a
// popup that dies silently gives the user nothing to act on.
func writeConsoleError(conn *websocket.Conn, format string, args ...interface{}) {
	message := fmt.Sprintf("\r\n\x1b[31m[console error]\x1b[0m %s\r\n", fmt.Sprintf(format, args...))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		log.WithError(err).Error("console: failed to send error to client")
	}
	conn.Close()
}

// HandleSpacesConsoleProxy bridges a websocket to a KVM space's serial
// console (`virsh console` under a pty), giving the browser access to the VM
// itself: login works with the owner's knot username and service password
// even when the agent never connected. Only the node that owns the VM can
// serve it — virsh talks to the local libvirt.
//
// The websocket is upgraded before any console precondition is checked, and
// every failure is written into the terminal as a [console error] line: a
// popup that dies silently gives the user nothing to act on.
func HandleSpacesConsoleProxy(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Check user access to the space
	if space.UserId != user.Id && !space.IsSharedWith(user.Id) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Upgrade before the console preconditions: from here on, every failure
	// is reported as a readable line in the terminal instead of a closed
	// popup.
	conn := util.UpgradeToWS(w, r)
	if conn == nil {
		log.Error("console: error while upgrading to websocket")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	fail := func(format string, args ...interface{}) {
		writeConsoleError(conn, format, args...)
	}

	if !user.HasPermission(model.PermissionUseWebTerminal) {
		fail("you do not have permission to use the web terminal")
		return
	}

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil || template == nil || !template.IsKvm() {
		fail("the space is not a KVM space — the console only applies to virtual machines")
		return
	}

	// The console is gated on the template's terminal feature like the agent
	// terminal: a template author who disabled terminal access must not have
	// it bypassed through the serial console.
	if !template.WithTerminal {
		fail("the template has the terminal feature disabled")
		return
	}

	if space.ContainerId == "" {
		fail("the space has not been started yet — start it first")
		return
	}

	// A space on another node has its VM on that node's libvirt. When that
	// node is a current cluster member, bridge through to its server. A node
	// that is no longer in the cluster (a node_id that was regenerated, or a
	// space that moved with a restored database) falls through to the local
	// libvirt — a VM that is actually here stays console-able, and the state
	// checks below produce a readable error when it is not.
	if forward, nodeId := service.ShouldForwardToNode(space.NodeId); forward {
		if endpoint, err := service.NodeAPIEndpoint(nodeId); err == nil {
			proxyConsoleToNode(conn, user, nodeId, spaceId, endpoint)
			return
		} else {
			log.Warn("console: space's node is not in the cluster, trying the local libvirt", "space_id", space.Id, "space_node", nodeId, "reason", err)
		}
	}

	state, err := kvm.NewClient().DomainState(r.Context(), space.ContainerId)
	if err != nil {
		log.WithError(err).Error("console: checking domain state", "domain", space.ContainerId)
		fail("failed to check the VM's state: %v", err)
		return
	}
	if state == "" {
		fail("the VM's domain was not found on this server and its node (%s) is not in the cluster", space.NodeId)
		return
	}
	if state != "running" {
		fail("the VM is not running (state: %s) — start the space first", state)
		return
	}

	if _, err := exec.LookPath("virsh"); err != nil {
		fail("virsh was not found in the server's PATH — the console requires running on the KVM node")
		return
	}

	// virsh console needs a tty on stdin; the pty is also what carries the
	// terminal size. Killing the process ends both directions. Its stderr
	// (e.g. "unable to find console device") lands in the server log, since
	// the pty only carries stdout.
	cmd := exec.Command("virsh", "--connect", "qemu:///system", "console", space.ContainerId)
	cmd.Stderr = os.Stderr
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.WithError(err).Error("console: failed to start virsh console", "domain", space.ContainerId)
		fail("failed to start the console: %v", err)
		return
	}

	consoleDone := make(chan struct{})

	// pty -> websocket. Closing the websocket on exit unblocks the reader
	// below and tells the browser the console ended.
	go func() {
		defer close(consoleDone)
		defer conn.Close()
		buffer := make([]byte, 2048)
		for {
			n, err := ptmx.Read(buffer)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		_ = cmd.Process.Kill()
	}()

	// websocket -> pty. The terminal client speaks the same wire protocol as
	// the agent terminal: keystrokes as text (or raw binary) frames, and
	// resize messages as a binary frame led by \x01 and a JSON size.
	go func() {
		for {
			mt, reader, err := conn.NextReader()
			if err != nil {
				break
			}

			data, err := io.ReadAll(reader)
			if err != nil {
				break
			}

			if mt == websocket.BinaryMessage && len(data) > 0 && data[0] == 1 {
				var size struct {
					Cols uint16 `json:"cols"`
					Rows uint16 `json:"rows"`
				}
				if err := json.Unmarshal(data[1:], &size); err == nil && size.Cols > 0 && size.Rows > 0 {
					_ = pty.Setsize(ptmx, &pty.Winsize{Rows: size.Rows, Cols: size.Cols})
				}
				continue
			}

			if _, err := ptmx.Write(data); err != nil {
				break
			}
		}
		_ = cmd.Process.Kill()
		_ = ptmx.Close()
	}()

	<-consoleDone
}

// proxyConsoleToNode bridges the browser's console websocket to the space's
// owning node, which runs the same handler against its local libvirt. Frames
// flow untouched in both directions, so the node's [console error] lines and
// the serial console itself reach the browser verbatim. Cluster credentials
// authenticate the inter-server connection (ApiAuth accepts X-Cluster-Key).
func proxyConsoleToNode(conn *websocket.Conn, user *model.User, nodeId, spaceId, endpoint string) {
	url := strings.Replace(endpoint, "http://", "ws://", 1)
	url = strings.Replace(url, "https://", "wss://", 1)
	url += "/proxy/spaces/" + spaceId + "/console"

	cfg := config.GetServerConfig()
	header := http.Header{}
	header.Set("X-Cluster-Key", cfg.Cluster.Key)
	header.Set("X-Cluster-User-Id", user.Id)

	dialer := &websocket.Dialer{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		HandshakeTimeout:  10 * time.Second,
		EnableCompression: false,
	}
	node, _, err := dialer.Dial(url, header)
	if err != nil {
		log.WithError(err).Error("console: failed to dial the owning node", "node", nodeId, "url", url)
		writeConsoleError(conn, "failed to reach the VM's node (%s): %v", nodeId, err)
		return
	}

	pumpWebsockets(conn, node)
}

// pumpWebsockets copies websocket frames between two connections until one
// side closes; closing both unblocks the copy in the other direction.
func pumpWebsockets(a, b *websocket.Conn) {
	done := make(chan struct{}, 2)
	pump := func(dst, src *websocket.Conn) {
		defer func() { done <- struct{}{} }()
		for {
			messageType, data, err := src.ReadMessage()
			if err != nil {
				return
			}
			if err := dst.WriteMessage(messageType, data); err != nil {
				return
			}
		}
	}

	go pump(a, b)
	go pump(b, a)

	<-done
	a.Close()
	b.Close()
}
