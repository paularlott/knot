package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

func handleConnect(conn net.Conn, msg *CommandMsg) error {
	server, token, err := agentClient.SendRequestToken()
	if err != nil {
		log.WithError(err).Error("Failed to send request token")
		return err
	}

	response := &ConnectResponse{
		Success: err == nil,
		Server:  server,
		Token:   token,
	}

	err = sendMsg(conn, CommandNil, &response)
	if err != nil {
		log.WithError(err).Error("Failed to send response")
		return err
	}

	return nil
}
