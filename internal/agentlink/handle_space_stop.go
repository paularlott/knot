package agentlink

import (
	"net"

	"github.com/rs/zerolog/log"
)

func handleSpaceStop(conn net.Conn, msg *CommandMsg) error {
	err := agentClient.SendSpaceStop()
	if err != nil {
		log.Error().Err(err).Msg("agent: Failed to send space stop")
		return err
	}

	return nil
}
