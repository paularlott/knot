package proxy

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

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

// HandleSpacesVNCProxy bridges a websocket to the VM's built-in QEMU VNC
// display — the framebuffer QEMU itself renders (bootloader, boot messages,
// desktop), served on the node's loopback. Nothing is installed in the
// guest: noVNC in the browser speaks RFB over the websocket, and this
// handler pipes those bytes to the local VNC TCP port. Like the console, it
// works when the guest's agent does not, and only the node that owns the VM
// can reach the display (cluster members bridge through to it).
func HandleSpacesVNCProxy(w http.ResponseWriter, r *http.Request) {
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

	// Upgrade before the preconditions: noVNC surfaces anything sent before
	// the close in its status line, so failures are visible in the viewer
	// instead of a silently dead window.
	conn := util.UpgradeToWS(w, r)
	if conn == nil {
		log.Error("vnc: error while upgrading to websocket")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	fail := func(format string, args ...interface{}) {
		message := fmt.Sprintf(format, args...)
		if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
			log.WithError(err).Error("vnc: failed to send error to client")
		}
		conn.Close()
	}

	if !user.HasPermission(model.PermissionUseVNC) {
		fail("no-vnc-permission: you do not have permission to use VNC")
		return
	}

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil || template == nil || !template.IsKvm() || !template.WithVNC {
		fail("no-vnc-display: the space's template does not expose the VM's display")
		return
	}

	if space.ContainerId == "" {
		fail("no-vnc-display: the space has not been started yet")
		return
	}

	// The display lives on the owning node's loopback; when that node is a
	// current cluster member bridge through to it. A node no longer in the
	// cluster falls through to the local libvirt — a VM that is actually
	// here stays reachable, and the checks below fail readably when it is
	// not.
	if forward, nodeId := service.ShouldForwardToNode(space.NodeId); forward {
		if endpoint, err := service.NodeAPIEndpoint(nodeId); err == nil {
			proxyVNCToNode(conn, user, nodeId, spaceId, endpoint)
			return
		} else {
			log.Warn("vnc: space's node is not in the cluster, trying the local libvirt", "space_id", space.Id, "space_node", nodeId, "reason", err)
		}
	}

	state, err := kvm.NewClient().DomainState(r.Context(), space.ContainerId)
	if err != nil {
		log.WithError(err).Error("vnc: checking domain state", "domain", space.ContainerId)
		fail("no-vnc-display: failed to check the VM's state: %v", err)
		return
	}
	if state == "" {
		fail("no-vnc-display: the VM's domain was not found on this server and its node is not in the cluster")
		return
	}
	if state != "running" {
		fail("no-vnc-display: the VM is not running (state: %s)", state)
		return
	}

	host, port, err := kvm.NewClient().VNCAddress(r.Context(), space.ContainerId)
	if err != nil {
		log.WithError(err).Error("vnc: resolving the domain's VNC display", "domain", space.ContainerId)
		fail("no-vnc-display: %v", err)
		return
	}
	log.Debug("vnc: display resolved", "domain", space.ContainerId, "address", net.JoinHostPort(host, fmt.Sprintf("%d", port)))

	vnc, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)), 10*time.Second)
	if err != nil {
		log.WithError(err).Error("vnc: dialing the domain's VNC port", "domain", space.ContainerId, "address", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
		fail("no-vnc-display: failed to reach the VM's VNC server: %v", err)
		return
	}

	log.Info("vnc: bridging the VM's display", "space_id", space.Id, "domain", space.ContainerId, "address", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	bridgeWebsocketToTCP(conn, vnc)
}

// bridgeWebsocketToTCP pipes the websocket's binary frames to the TCP
// stream and vice versa until either side closes; RFB is a byte-stream
// protocol, so the websocket message framing is transparent to it.
func bridgeWebsocketToTCP(conn *websocket.Conn, tcp net.Conn) {
	done := make(chan struct{}, 2)

	// websocket -> TCP (client input: keyboard, pointer)
	go func() {
		defer func() { done <- struct{}{} }()
		var relayed int64
		defer func() { log.Debug("vnc: client input pump ended", "bytes", relayed) }()
		for {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if messageType == websocket.TextMessage {
				// Control frames from the page (errors aside) are not part
				// of RFB; ignore them.
				continue
			}
			if _, err := tcp.Write(data); err != nil {
				return
			}
			relayed += int64(len(data))
		}
	}()

	// TCP -> websocket (display updates)
	go func() {
		defer func() { done <- struct{}{} }()
		var relayed int64
		firstByte := true
		defer func() { log.Debug("vnc: display pump ended", "bytes", relayed) }()
		buffer := make([]byte, 32768)
		for {
			n, err := tcp.Read(buffer)
			if n > 0 {
				if firstByte {
					log.Debug("vnc: first bytes from the display", "bytes", n, "prefix", fmt.Sprintf("%q", buffer[:min(n, 12)]))
					firstByte = false
				}
				if werr := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); werr != nil {
					return
				}
				relayed += int64(n)
			}
			if err != nil {
				return
			}
		}
	}()

	<-done
	conn.Close()
	tcp.Close()
}

// proxyVNCToNode bridges the browser's VNC websocket to the space's owning
// node, which runs this same handler against its local display. Frames flow
// untouched in both directions; cluster credentials authenticate the
// inter-server connection.
func proxyVNCToNode(conn *websocket.Conn, user *model.User, nodeId, spaceId, endpoint string) {
	url := strings.Replace(endpoint, "http://", "ws://", 1)
	url = strings.Replace(url, "https://", "wss://", 1)
	url += "/proxy/spaces/" + spaceId + "/vnc"

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
		log.WithError(err).Error("vnc: failed to dial the owning node", "node", nodeId, "url", url)
		writeVNCError(conn, "no-vnc-display: failed to reach the VM's node (%s): %v", nodeId, err)
		return
	}

	pumpWebsockets(conn, node)
}

func writeVNCError(conn *websocket.Conn, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		log.WithError(err).Error("vnc: failed to send error to client")
	}
	conn.Close()
}
