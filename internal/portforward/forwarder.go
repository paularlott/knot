package portforward

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/paularlott/knot/internal/wsconn"

	"github.com/paularlott/knot/internal/log"
)

// DirectDialFunc attempts to forward a connection directly to a peer agent.
// Returns nil if the connection was fully handled (caller should close the
// local conn). Returns non-nil error if direct failed and the caller should
// fall back to relay.
type DirectDialFunc func(ctx context.Context, conn net.Conn, space string, remotePort uint16) error

var (
	directDialer   DirectDialFunc
	directDialerMu sync.RWMutex
)

// SetDirectDialer installs the global direct-connection function. Called once
// at agent startup; the port forwarder checks it for every accepted connection.
func SetDirectDialer(fn DirectDialFunc) {
	directDialerMu.Lock()
	defer directDialerMu.Unlock()
	directDialer = fn
}

func getDirectDialer() DirectDialFunc {
	directDialerMu.RLock()
	defer directDialerMu.RUnlock()
	return directDialer
}

// Shared state for port forwards (used by both agentlink and mux session handlers)
var (
	forwardsMux     sync.RWMutex
	forwards        = make(map[uint16]*ForwardInfo)
	persistentPorts = make(map[uint16]bool)
)

// ForwardInfo holds information about an active port forward
type ForwardInfo struct {
	LocalPort  uint16
	Space      string
	RemotePort uint16
	Mode       string // "relay" or "direct"
	Cancel     context.CancelFunc
	Listener   net.Listener
}

// StartForward starts a new port forward and returns the info
func StartForward(localPort, remotePort uint16, space string, cancel context.CancelFunc) *ForwardInfo {
	forwardsMux.Lock()
	defer forwardsMux.Unlock()

	info := &ForwardInfo{
		LocalPort:  localPort,
		Space:      space,
		RemotePort: remotePort,
		Mode:       "", // empty = try direct on first connection; set to "relay" after failure
		Cancel:     cancel,
	}
	forwards[localPort] = info
	return info
}

// StopForward stops and removes a port forward
func StopForward(localPort uint16) {
	forwardsMux.Lock()
	defer forwardsMux.Unlock()

	if fwd, exists := forwards[localPort]; exists {
		fwd.Cancel()
		if fwd.Listener != nil {
			fwd.Listener.Close()
		}
		delete(forwards, localPort)
		delete(persistentPorts, localPort)
	}
}

// GetForward returns info about a specific port forward
func GetForward(localPort uint16) (*ForwardInfo, bool) {
	forwardsMux.RLock()
	defer forwardsMux.RUnlock()

	fwd, exists := forwards[localPort]
	return fwd, exists
}

// ListForwards returns all active port forwards
func ListForwards() []*ForwardInfo {
	forwardsMux.RLock()
	defer forwardsMux.RUnlock()

	result := make([]*ForwardInfo, 0, len(forwards))
	for _, fwd := range forwards {
		result = append(result, fwd)
	}
	return result
}

// IsPortForwarded checks if a port is already being forwarded
func IsPortForwarded(localPort uint16) bool {
	forwardsMux.RLock()
	defer forwardsMux.RUnlock()

	_, exists := forwards[localPort]
	return exists
}

// StoreListener stores the listener for a port forward (called after listener is created)
func StoreListener(localPort uint16, listener net.Listener) {
	forwardsMux.Lock()
	defer forwardsMux.Unlock()

	if fwd, exists := forwards[localPort]; exists {
		fwd.Listener = listener
	}
}

// MarkPersistent marks a port forward as persistent.
func MarkPersistent(localPort uint16) {
	forwardsMux.Lock()
	defer forwardsMux.Unlock()
	persistentPorts[localPort] = true
}

// UnmarkPersistent removes the persistent mark from a port forward.
func UnmarkPersistent(localPort uint16) {
	forwardsMux.Lock()
	defer forwardsMux.Unlock()
	delete(persistentPorts, localPort)
}

// IsPersistent checks if a port forward is marked as persistent.
func IsPersistent(localPort uint16) bool {
	forwardsMux.RLock()
	defer forwardsMux.RUnlock()
	return persistentPorts[localPort]
}

// ResetModeForSpace sets the mode to "direct" on all forwards targeting the
// given space, called when a fresh PeerIntroduce arrives with an updated peer
// address. The forwarder will use direct; if it fails it switches to "relay".
func ResetModeForSpace(space string) {
	forwardsMux.Lock()
	defer forwardsMux.Unlock()
	for _, fwd := range forwards {
		if fwd.Space == space {
			fwd.Mode = "direct"
		}
	}
}

// RunTCPForwarderViaAgentWithContext runs a TCP forwarder via the agent proxy server
func RunTCPForwarderViaAgentWithContext(ctx context.Context, proxyServerURL, listen, space string, port int, token string, skipTLSVerify bool) net.Listener {
	logger := log.WithGroup("tcp")

	// Convert http/https to ws/wss
	wsURL := proxyServerURL
	if strings.HasPrefix(proxyServerURL, "https://") {
		wsURL = "wss://" + proxyServerURL[8:]
	} else if strings.HasPrefix(proxyServerURL, "http://") {
		wsURL = "ws://" + proxyServerURL[7:]
	}

	logger.Info("port forward listening", "local", listen, "space", space, "port", port)
	return forwardTCPWithContext(ctx, fmt.Sprintf("%s/proxy/spaces/%s/port/%d", wsURL, space, port), token, listen, skipTLSVerify)
}

func forwardTCPWithContext(ctx context.Context, dialURL, token, listen string, skipTLSVerify bool) net.Listener {
	logger := log.WithGroup("tcp")
	tcpConnection, err := net.Listen("tcp", listen)
	if err != nil {
		logger.WithError(err).Error("error while opening local port")
		return nil
	}

	// Extract the local port for ForwardInfo lookups
	_, portStr, _ := net.SplitHostPort(listen)
	localPort, _ := strconv.Atoi(portStr)

	go func() {
		defer tcpConnection.Close()

		// Include auth header if given
		var header http.Header
		if token != "" {
			header = http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", token)}}
		} else {
			header = nil
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			tcpConn, err := tcpConnection.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					logger.Debug("listener closed", "error", err)
					return
				}
			}

			// Try direct peer connection first — but only if this forward
			// hasn't already been switched to relay mode after a failure.
			// ResetModeForSpace (called when a fresh PeerIntroduce arrives)
			// clears the mode back to "" so direct is retried.
			if dialer := getDirectDialer(); dialer != nil {
				if fwd, ok := GetForward(uint16(localPort)); ok && fwd.Mode != "relay" {
					if err := dialer(ctx, tcpConn, fwd.Space, fwd.RemotePort); err == nil {
						if fwd.Mode != "direct" {
							logger.Info("using direct", "space", fwd.Space, "local_port", localPort)
						}
						fwd.Mode = "direct"
						tcpConn.Close()
						continue
					}
					// Direct failed — switch to relay mode for subsequent connections
					if fwd.Mode != "relay" {
						logger.Warn("direct failed, using relay", "space", fwd.Space, "local_port", localPort)
					}
					fwd.Mode = "relay"
				}
			}

			// Relay via server WebSocket
			dialer := websocket.DefaultDialer
			dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: skipTLSVerify}
			dialer.HandshakeTimeout = 5 * time.Second
			wsConn, response, err := dialer.Dial(dialURL, header)
			if err != nil {
				if response != nil && response.StatusCode == http.StatusUnauthorized {
					logger.Error("tcp", response.Status)
				} else if response != nil && response.StatusCode == http.StatusForbidden {
					logger.Error("proxy of remote port is not allowed")
				} else {
					logger.WithError(err).Error("error while dialing:")
				}

				tcpConn.Close()
				continue
			}

			conn := wsconn.New(wsConn)

			go func() {
				// copy data between code server and server
				var once sync.Once
				closeBoth := func() {
					conn.Close()
					tcpConn.Close()
				}

				// Copy from client to tunnel
				go func() {
					_, _ = io.Copy(conn, tcpConn)
					once.Do(closeBoth)
				}()

				// Copy from tunnel to client
				_, _ = io.Copy(tcpConn, conn)
				once.Do(closeBoth)
			}()
		}
	}()

	return tcpConnection
}
