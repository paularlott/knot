package agentlink

import (
	"context"
	"net"
	"os"

	"github.com/paularlott/knot/internal/agentapi/agent_client"

	"github.com/rs/zerolog/log"
)

const (
	commandSocketPath = ".knot"
	commandSocket     = "agent.sock"
)

var (
	cancelContext context.Context
	cancelFunc    context.CancelFunc
	agentClient   *agent_client.AgentClient
)

func StartCommandSocket(agentClientObj *agent_client.AgentClient) {
	agentClient = agentClientObj

	log.Info().Msg("agent: Starting command socket")

	// Create the folder for the socket in the user's home directory
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal().Err(err).Msg("agent: Failed to get home directory")
	}

	err = os.MkdirAll(home+"/"+commandSocketPath, os.ModePerm)
	if err != nil {
		log.Fatal().Err(err).Msg("agent: Failed to create socket directory")
	}

	cancelContext, cancelFunc = context.WithCancel(context.Background())
	go func() {
		// Listen on the socket
		socketPath := home + "/" + commandSocketPath + "/" + commandSocket
		os.Remove(socketPath) // Remove any existing socket
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			log.Fatal().Err(err).Msg("agent: Failed to listen on socket")
		}
		os.Chmod(socketPath, 0700)
		defer func() {
			listener.Close()
			os.Remove(socketPath)
			log.Info().Msg("agent: Command socket listener stopped")
		}()

		for {
			select {
			case <-cancelContext.Done():
				log.Info().Msg("agent: Command socket listener stopped by context")
				return
			default:
				conn, err := listener.Accept()
				if err != nil {
					if ne, ok := err.(net.Error); ok && ne.Timeout() {
						continue // check context again
					}
					log.Error().Err(err).Msg("Failed to accept connection")
					continue
				}

				go handleCommandConnection(conn)
			}
		}
	}()
}

func StopCommandSocket() {
	cancelFunc()
}

func handleCommandConnection(conn net.Conn) {
	defer conn.Close()

	msg, err := receiveMsg(conn)
	if err != nil {
		log.Error().Err(err).Msg("agent: Failed to receive message")
		return
	}

	switch msg.Command {
	case CommandConnect:
		handleConnect(conn, msg)

	case CommandSpaceNote:
		handleSpaceNote(conn, msg)
	}
}
