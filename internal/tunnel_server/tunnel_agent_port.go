package tunnel_server

import (
	"fmt"
	"io"
	"net"

	"github.com/paularlott/knot/internal/log"
)

func TunnelAgentPort(agentSessionId string, port uint16, conn net.Conn) {
	logger := log.WithGroup("tunnel")
	tunnelName := fmt.Sprintf("--%s:%d", agentSessionId, port)

	// Get the tunnel session
	tunnelMutex.RLock()
	session, ok := tunnels[tunnelName]
	tunnelMutex.RUnlock()
	if !ok || session.tunnelType != PortTunnel {
		logger.Error("not found")
		return
	}

	// Open a new stream to the tunnel client
	clientStream, err := session.muxSession.Open()
	if err != nil {
		return
	}
	defer clientStream.Close()

	// Write a byte with a value of 1 so the client knows this is a new connection
	_, err = clientStream.Write([]byte{1})
	if err != nil {
		return
	}

	// Bidirectional copy
	go io.Copy(conn, clientStream)
	io.Copy(clientStream, conn)
}
