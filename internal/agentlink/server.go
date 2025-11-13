package agentlink

import (
	"context"
	"net"
	"os"

	"github.com/paularlott/knot/internal/agentapi/agent_client"

	"github.com/paularlott/knot/internal/log"
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

	log.Info("Starting command socket")

	// Create the folder for the socket in the user's home directory
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Failed to get home directory", "error", err)
	}

	err = os.MkdirAll(home+"/"+commandSocketPath, os.ModePerm)
	if err != nil {
		log.Fatal("Failed to create socket directory", "error", err)
	}

	cancelContext, cancelFunc = context.WithCancel(context.Background())
	go func() {
		// Listen on the socket
		socketPath := home + "/" + commandSocketPath + "/" + commandSocket
		os.Remove(socketPath) // Remove any existing socket
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			log.Fatal("Failed to listen on socket", "error", err)
		}
		os.Chmod(socketPath, 0700)
		defer func() {
			listener.Close()
			os.Remove(socketPath)
			log.Info("Command socket listener stopped")
		}()

		for {
			select {
			case <-cancelContext.Done():
				log.Info("Command socket listener stopped by context")
				return
			default:
				conn, err := listener.Accept()
				if err != nil {
					if ne, ok := err.(net.Error); ok && ne.Timeout() {
						continue // check context again
					}
					log.WithError(err).Error("Failed to accept connection")
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
		log.WithError(err).Error("Failed to receive message")
		return
	}

	switch msg.Command {
	case CommandConnect:
		handleConnect(conn, msg)

	case CommandSpaceNote:
		handleSpaceNote(conn, msg)

	case CommandSpaceVar:
		handleSpaceVar(conn, msg)

	case CommandSpaceGetVar:
		handleSpaceGetVar(conn, msg)

	case CommandSpaceStop:
		handleSpaceStop(conn, msg)

	case CommandSpaceRestart:
		handleSpaceRestart(conn, msg)
	}
}
