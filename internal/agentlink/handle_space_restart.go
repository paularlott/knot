package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

func handleSpaceRestart(conn net.Conn, msg *CommandMsg) error {
	err := agentClient.SendSpaceRestart()
	if err != nil {
		log.WithError(err).Error("Failed to send space restart")
		return err
	}

	return nil
}
