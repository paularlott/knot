package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

func handleSpaceStop(conn net.Conn, msg *CommandMsg) error {
	err := agentClient.SendSpaceStop()
	if err != nil {
		log.WithError(err).Error("Failed to send space stop")
		return err
	}

	return nil
}
