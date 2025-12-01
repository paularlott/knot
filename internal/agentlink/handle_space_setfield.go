package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

func handleSpaceSetField(conn net.Conn, msg *CommandMsg) error {
	var varReq SpaceFieldRequest
	err := msg.Unmarshal(&varReq)
	if err != nil {
		log.WithError(err).Error("Failed to unmarshal space field")
		return err
	}

	err = agentClient.SendSpaceVar(varReq.Name, varReq.Value)
	if err != nil {
		log.WithError(err).Error("Failed to send space field")
		return err
	}

	return nil
}
