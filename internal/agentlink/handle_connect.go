package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

// handleConnect returns the agent's stored authentication credentials.
// These credentials (token, server URL, space ID) are received from the server
// during agent registration and stored for the lifetime of the agent process.
func handleConnect(conn net.Conn, msg *CommandMsg) error {
	// Get connection info from agent client (stored during registration)
	server := agentClient.GetServerURL()
	token := agentClient.GetAgentToken()
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
