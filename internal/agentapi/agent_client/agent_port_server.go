package agent_client

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/paularlott/knot/internal/log"
)

func (s *agentServer) agentPortListenAndServe(stream net.Conn, port uint16) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a listener on the specified port
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Error("failed to create listener for port", "error", err, "port", port)
		return
	}
	defer listener.Close()

	go func() {
		// Reading from stream until EOF or error indicates the stream has closed
		buf := make([]byte, 1)
		_, err := stream.Read(buf)
		if err != nil {
			log.WithError(err).Debug("tunnel control stream closed:")
			cancel()
		}
	}()

	// Handle incoming connections
	connectionChan := make(chan net.Conn)
	errorChan := make(chan error)

	go func() {
		for {
			clientConn, err := listener.Accept()
			if err != nil {
				errorChan <- err
				return
			}
			connectionChan <- clientConn
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case err := <-errorChan:
			log.Error("error accepting connection on port", "error", err, "port", port)
			continue

		case clientConn := <-connectionChan:
			// For each connection, open a new stream to the server
			tunnelStream, err := s.muxSession.OpenStream()
			if err != nil {
				log.WithError(err).Error("failed to open mux stream for tunnel")
				clientConn.Close()
				continue
			}

			// Send tunnel connection notification
			if err := msg.WriteCommand(tunnelStream, msg.CmdType(msg.CmdTunnelPortConnection)); err != nil {
				log.WithError(err).Error("error writing tunnel connection command")
				tunnelStream.Close()
				clientConn.Close()
				continue
			}

			// Send port info
			if err := msg.WriteMessage(tunnelStream, &msg.TcpPort{
				Port: port,
			}); err != nil {
				log.WithError(err).Error("error writing tunnel port info")
				tunnelStream.Close()
				clientConn.Close()
				continue
			}

			// Bidirectional copy
			go func() {
				defer log.Debug("closed tunnel between  and", "tunnel", clientConn.RemoteAddr(), port)

				var once sync.Once
				closeBoth := func() {
					clientConn.Close()
					tunnelStream.Close()
				}

				log.Debug("established tunnel between  and", "tunnel", clientConn.RemoteAddr(), port)

				// Copy from client to tunnel
				go func() {
					_, _ = io.Copy(tunnelStream, clientConn)
					once.Do(closeBoth)
				}()

				// Copy from tunnel to client
				_, _ = io.Copy(clientConn, tunnelStream)
				once.Do(closeBoth)
			}()
		}
	}
}
