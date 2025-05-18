package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/agentapi/agent_client"

	"github.com/rs/zerolog/log"
)

func handleConnect(conn net.Conn, msg *CommandMsg) error {
	server, token, err := agent_client.SendRequestToken()
	if err != nil {
		log.Error().Err(err).Msg("agent: Failed to send request token")
		return err
	}

	response := &ConnectResponse{
		Success: err == nil,
		Server:  server,
		Token:   token,
	}

	err = sendMsg(conn, CommandNil, &response)
	if err != nil {
		log.Error().Err(err).Msg("agent: Failed to send response")
		return err
	}

	return nil
}
