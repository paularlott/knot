package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

func handleSpaceVar(conn net.Conn, msg *CommandMsg) error {
	var varReq SpaceVarRequest
	err := msg.Unmarshal(&varReq)
	if err != nil {
		log.WithError(err).Error("Failed to unmarshal space var")
		return err
	}

	err = agentClient.SendSpaceVar(varReq.Name, varReq.Value)
	if err != nil {
		log.WithError(err).Error("Failed to send space var")
		return err
	}

	return nil
}
