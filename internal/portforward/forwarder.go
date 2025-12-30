package portforward

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/paularlott/knot/internal/wsconn"

	"github.com/paularlott/knot/internal/log"
)

// Shared state for port forwards (used by both agentlink and mux session handlers)
var (
	forwardsMux sync.RWMutex
	forwards    = make(map[uint16]*ForwardInfo)
)

// ForwardInfo holds information about an active port forward
type ForwardInfo struct {
	LocalPort  uint16
	Space      string
	RemotePort uint16
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

	logger.Info("connecting to agent via server at", "proxyServerURL", wsURL)
	return forwardTCPWithContext(ctx, fmt.Sprintf("%s/proxy/spaces/%s/port/%d", wsURL, space, port), token, listen, skipTLSVerify)
}

func forwardTCPWithContext(ctx context.Context, dialURL, token, listen string, skipTLSVerify bool) net.Listener {
	logger := log.WithGroup("tcp")
	tcpConnection, err := net.Listen("tcp", listen)
	if err != nil {
		logger.WithError(err).Error("error while opening local port")
		return nil
	}

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
				logger.WithError(err).Error("could not accept the connection:")
				return
			}

			// Create websocket connection
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
