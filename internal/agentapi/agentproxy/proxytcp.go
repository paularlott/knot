// Package agentproxy provides self-contained TCP/TLS stream-to-local-port
// proxying. It is a leaf package so that both tunnel_server (the tunnel client)
// and agent_client can depend on it without forming an import cycle.
package agentproxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/log"
)

// ProxyTcp dials 127.0.0.1:port and copies data bidirectionally to/from
// stream. onActivity, when non-nil, is called (from either copy direction)
// each time bytes flow and periodically while the stream stays open, so
// callers can treat an open session — not just a chatty one — as activity.
func ProxyTcp(stream net.Conn, port string, onActivity func()) {
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", port))
	if err != nil {
		log.WithError(err).Error("failed to connect to code server")
		return
	}
	defer conn.Close()

	copyBidirectionally(conn, stream, onActivity)
}

// ProxyTcpTls dials 127.0.0.1:port over TLS and copies data bidirectionally.
func ProxyTcpTls(stream net.Conn, port, serverName string, skipTLSVerify bool, onActivity func()) {
	conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", port), &tls.Config{
		InsecureSkipVerify: skipTLSVerify,
		ServerName:         serverName,
	})
	if err != nil {
		log.WithError(err).Error("failed to connect to code server")
		return
	}
	defer conn.Close()

	copyBidirectionally(conn, stream, onActivity)
}

// copyBidirectionally copies data between conn and stream, closing conn when
// either direction finishes.
func copyBidirectionally(conn net.Conn, stream net.Conn, onActivity func()) {
	// copy data between code server and server
	var once sync.Once
	closeConn := func() {
		conn.Close()
	}

	// An open-but-silent session (idle SSH connection, open terminal,
	// websocket) sends no bytes to mark, but still counts as the space being
	// in use: mark activity periodically until the session ends.
	if onActivity != nil {
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			ticker := time.NewTicker(activityHeartbeatInterval)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					onActivity()
				}
			}
		}()
	}

	// Copy from client to tunnel
	go func() {
		copyActivity(conn, stream, onActivity)
		once.Do(closeConn)
	}()

	// Copy from tunnel to client
	copyActivity(stream, conn, onActivity)
	once.Do(closeConn)
}

// activityHeartbeatInterval is how often an open proxied session re-marks
// activity. Well under the smallest sane idle timeout (minutes), so an open
// session reliably keeps the space alive.
const activityHeartbeatInterval = 30 * time.Second

// copyActivity is io.Copy with an optional per-chunk activity callback.
func copyActivity(dst io.Writer, src io.Reader, onActivity func()) {
	buffer := make([]byte, 32*1024)
	for {
		n, err := src.Read(buffer)
		if n > 0 {
			if onActivity != nil {
				onActivity()
			}
			if _, werr := dst.Write(buffer[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
