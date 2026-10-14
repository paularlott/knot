package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

// handleConnect returns the credentials processes in the space use: the
// server URL, the space ID and the token for processes, never the agent's
// own token. They are received from the server during agent registration
// and stored for the lifetime of the agent process.
func handleConnect(conn net.Conn, msg *CommandMsg) error {
	// Get connection info from agent client (stored during registration)
	server := agentClient.GetServerURL()
	token := agentClient.GetAgentCliToken()
	spaceId := agentClient.GetSpaceId()

	response := &ConnectResponse{
		Success: server != "" && token != "",
		Server:  server,
		Token:   token,
		SpaceID: spaceId,
	}

	err := sendMsg(conn, CommandNil, &response)
	if err != nil {
		log.WithError(err).Error("Failed to send response")
		return err
	}

	return nil
}
