package portforward

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
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
	mode       string // "relay" or "direct" — use GetMode/SetMode (thread-safe)
	Cancel     context.CancelFunc
	Listener   net.Listener

	// Throttle settings (runtime only, not persisted)
	throttleMu  sync.RWMutex
	latencyMs   int
	jitterMs    int
	bandwidthKB int // KB/s, 0 = unlimited
}

// GetMode returns the current connection mode (thread-safe).
func (f *ForwardInfo) GetMode() string {
	forwardsMux.RLock()
	defer forwardsMux.RUnlock()
	return f.mode
}

// SetMode sets the connection mode (thread-safe).
func (f *ForwardInfo) SetMode(mode string) {
	forwardsMux.Lock()
	defer forwardsMux.Unlock()
	f.mode = mode
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

// SetThrottle applies latency, jitter, and/or bandwidth limits to an existing
// forward. All values are optional; pass 0 to clear any individual setting.
// A new limiter is created on each call.
func (f *ForwardInfo) SetThrottle(latencyMs, jitterMs, bandwidthKB int) {
	f.throttleMu.Lock()
	defer f.throttleMu.Unlock()
	f.latencyMs = latencyMs
	f.jitterMs = jitterMs
	f.bandwidthKB = bandwidthKB
}

// GetThrottle returns the current throttle settings.
func (f *ForwardInfo) GetThrottle() (latencyMs, jitterMs, bandwidthKB int) {
	f.throttleMu.RLock()
	defer f.throttleMu.RUnlock()
	return f.latencyMs, f.jitterMs, f.bandwidthKB
}

// HasThrottle reports whether any throttle setting is active.
func (f *ForwardInfo) HasThrottle() bool {
	f.throttleMu.RLock()
	defer f.throttleMu.RUnlock()
	return f.latencyMs > 0 || f.jitterMs > 0 || f.bandwidthKB > 0
}

// throttledWriter wraps an io.Writer with optional latency, jitter, and
// bandwidth limiting. Reads settings from ForwardInfo on every Write so
// changes via SetThrottle take effect immediately.
type throttledWriter struct {
	dest io.Writer
	fwd  *ForwardInfo
	rng  *rand.Rand
}

func newThrottledWriter(dest io.Writer, fwd *ForwardInfo) io.Writer {
	if fwd == nil || !fwd.HasThrottle() {
		return dest
	}
	return &throttledWriter{dest: dest, fwd: fwd, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

// NewThrottledWriter is the exported version for use by the direct dial path.
func NewThrottledWriter(dest io.Writer, fwd *ForwardInfo) io.Writer {
	return newThrottledWriter(dest, fwd)
}

// FindForwardBySpace returns the first forward targeting the given space.
func FindForwardBySpace(space string) *ForwardInfo {
	forwardsMux.RLock()
	defer forwardsMux.RUnlock()
	for _, fwd := range forwards {
		if fwd.Space == space {
			return fwd
		}
	}
	return nil
}

func (w *throttledWriter) Write(p []byte) (int, error) {
	w.fwd.throttleMu.RLock()
	latencyMs := w.fwd.latencyMs
	jitterMs := w.fwd.jitterMs
	bandwidthKB := w.fwd.bandwidthKB
	w.fwd.throttleMu.RUnlock()

	// Bandwidth limiting: sleep proportional to data size
	if bandwidthKB > 0 {
		bps := bandwidthKB * 1024
		sleepDuration := time.Duration(len(p)) * time.Second / time.Duration(bps)
		if sleepDuration > 0 {
			time.Sleep(sleepDuration)
		}
	}

	// Latency + jitter
	if latencyMs > 0 {
		delay := time.Duration(latencyMs) * time.Millisecond
		if jitterMs > 0 {
			jitter := time.Duration(jitterMs) * time.Millisecond
			delay += time.Duration(w.rng.Int63n(int64(jitter*2))) - jitter
			if delay < 0 {
				delay = 0
			}
		}
		time.Sleep(delay)
	}

	return w.dest.Write(p)
}

// ResetModeForSpace sets the mode to "direct" on all forwards targeting the
// given space, called when a fresh PeerIntroduce arrives with an updated peer
// address. The forwarder will use direct; if it fails it switches to "relay".
func ResetModeForSpace(space string) {
	forwardsMux.Lock()
	defer forwardsMux.Unlock()
	for _, fwd := range forwards {
		if fwd.Space == space {
			fwd.mode = "direct"
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
			if dialer := getDirectDialer(); dialer != nil {
				if fwd, ok := GetForward(uint16(localPort)); ok && fwd.GetMode() != "relay" {
					if err := dialer(ctx, tcpConn, fwd.Space, fwd.RemotePort); err == nil {
						if fwd.GetMode() != "direct" {
							logger.Info("using direct", "space", fwd.Space, "local_port", localPort)
						}
						fwd.SetMode("direct")
						tcpConn.Close()
						continue
					}
					// Direct failed — switch to relay mode for subsequent connections
					if fwd.GetMode() != "relay" {
						logger.Warn("direct failed, using relay", "space", fwd.Space, "local_port", localPort)
					}
					fwd.SetMode("relay")
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

			// Look up the ForwardInfo for throttle settings
			var fwd *ForwardInfo
			if f, ok := GetForward(uint16(localPort)); ok {
				fwd = f
			}

			go func() {
				// copy data between code server and server
				var once sync.Once
				closeBoth := func() {
					conn.Close()
					tcpConn.Close()
				}

				// Copy from client to tunnel (throttled if configured)
				toTunnel := newThrottledWriter(conn, fwd)
				go func() {
					_, _ = io.Copy(toTunnel, tcpConn)
					once.Do(closeBoth)
				}()

				// Copy from tunnel to client (throttled if configured)
				toClient := newThrottledWriter(tcpConn, fwd)
				_, _ = io.Copy(toClient, conn)
				once.Do(closeBoth)
			}()
		}
	}()

	return tcpConnection
}
