package agentlink

import (
	"net"

	"github.com/rs/zerolog/log"
)

func handleSpaceRestart(conn net.Conn, msg *CommandMsg) error {
	err := agentClient.SendSpaceRestart()
	if err != nil {
		log.Error().Err(err).Msg("agent: Failed to send space restart")
		return err
	}

	return nil
}
