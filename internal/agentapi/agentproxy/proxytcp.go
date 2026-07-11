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

	"github.com/paularlott/knot/internal/log"
)

// ProxyTcp dials 127.0.0.1:port and copies data bidirectionally to/from stream.
func ProxyTcp(stream net.Conn, port string) {
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", port))
	if err != nil {
		log.WithError(err).Error("failed to connect to code server")
		return
	}
	defer conn.Close()

	// copy data between code server and server
	var once sync.Once
	closeConn := func() {
		conn.Close()
	}

	// Copy from client to tunnel
	go func() {
		_, _ = io.Copy(conn, stream)
		once.Do(closeConn)
	}()

	// Copy from tunnel to client
	_, _ = io.Copy(stream, conn)
	once.Do(closeConn)
}

// ProxyTcpTls dials 127.0.0.1:port over TLS and copies data bidirectionally.
func ProxyTcpTls(stream net.Conn, port, serverName string, skipTLSVerify bool) {
	conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", port), &tls.Config{
		InsecureSkipVerify: skipTLSVerify,
		ServerName:         serverName,
	})
	if err != nil {
		log.WithError(err).Error("failed to connect to code server")
		return
	}
	defer conn.Close()

	// copy data between code server and server
	var once sync.Once
	closeConn := func() {
		conn.Close()
	}

	// Copy from client to tunnel
	go func() {
		_, _ = io.Copy(conn, stream)
		once.Do(closeConn)
	}()

	// Copy from tunnel to client
	_, _ = io.Copy(stream, conn)
	once.Do(closeConn)
}
