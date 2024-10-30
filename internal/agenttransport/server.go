package agenttransport

import (
	"crypto/tls"
	"fmt"
	"net"

	"github.com/hashicorp/yamux"
	"github.com/rs/zerolog/log"
)

type AgentTransportServer struct {
	listen    string
	tlsConfig *tls.Config
}

func NewAgentTransportServer(listen string, tlsConfig *tls.Config) *AgentTransportServer {
	return &AgentTransportServer{
		listen:    listen,
		tlsConfig: tlsConfig,
	}
}

func (server *AgentTransportServer) ListenAndServe() {
	log.Info().Msgf("server: listening for agents on: %s", server.listen)

	go func() {

		// Open the agent listener
		var listener net.Listener
		var err error

		if server.tlsConfig == nil {
			listener, err = net.Listen("tcp", server.listen)
		} else {
			listener, err = tls.Listen("tcp", server.listen, server.tlsConfig)
		}
		if err != nil {
			log.Fatal().Msgf("Error starting agent listener: %v", err)
		}
		defer listener.Close()

		// Run forever listening for new connections
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Error().Msgf("Error accepting connection: %v", err)
				continue
			}

			// Start a new goroutine to handle the connection
			go server.handleConnection(conn)
		}
	}()
}

func (server *AgentTransportServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	// TODO Wait for & read the connection header, UUID

	// TODO Save the agent against the space

	// Create a yamux session
	session, err := yamux.Server(conn, nil)
	if err != nil {
		log.Error().Msgf("Error creating yamux session: %v", err)
		return
	}

	log.Debug().Msgf("New session for %s", conn.RemoteAddr())

	// Loop forever accepting new streams
	for {
		stream, err := session.Accept()
		if err != nil {
			log.Error().Msgf("Error accepting stream: %v", err)
			return
		}

		fmt.Println("Stream accepted", stream)

		// Start a new goroutine to handle the stream
		//    go server.handleStream(stream)
	}
}
