package agent_client

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/rs/zerolog/log"
)

func (s *agentServer) agentPortListenAndServe(stream net.Conn, port uint16) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a listener on the specified port
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Error().Err(err).Msgf("agent: failed to create listener for port %d", port)
		return
	}
	defer listener.Close()

	go func() {
		// Reading from stream until EOF or error indicates the stream has closed
		buf := make([]byte, 1)
		_, err := stream.Read(buf)
		if err != nil {
			log.Debug().Msgf("agent: tunnel control stream closed: %v", err)
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
			log.Error().Err(err).Msgf("agent: error accepting connection on port %d", port)
			continue

		case clientConn := <-connectionChan:
			// For each connection, open a new stream to the server
			tunnelStream, err := s.muxSession.OpenStream()
			if err != nil {
				log.Error().Err(err).Msgf("agent: failed to open mux stream for tunnel")
				clientConn.Close()
				continue
			}

			// Send tunnel connection notification
			if err := msg.WriteCommand(tunnelStream, msg.CmdType(msg.CmdTunnelPortConnection)); err != nil {
				log.Error().Err(err).Msgf("agent: error writing tunnel connection command")
				tunnelStream.Close()
				clientConn.Close()
				continue
			}

			// Send port info
			if err := msg.WriteMessage(tunnelStream, &msg.TcpPort{
				Port: port,
			}); err != nil {
				log.Error().Err(err).Msgf("agent: error writing tunnel port info")
				tunnelStream.Close()
				clientConn.Close()
				continue
			}

			// Bidirectional copy
			go func() {
				defer clientConn.Close()
				defer tunnelStream.Close()

				// Copy from client to tunnel
				go io.Copy(tunnelStream, clientConn)

				// Copy from tunnel to client
				io.Copy(clientConn, tunnelStream)
			}()
		}
	}
}
