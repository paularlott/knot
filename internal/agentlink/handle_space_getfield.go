package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

func handleSpaceGetField(conn net.Conn, msg *CommandMsg) error {
	var varReq SpaceGetFieldRequest
	err := msg.Unmarshal(&varReq)
	if err != nil {
		log.WithError(err).Error("Failed to unmarshal space get field")
		return err
	}

	value, err := agentClient.SendSpaceGetVar(varReq.Name)
	if err != nil {
		log.WithError(err).Error("Failed to get space field")
		return err
	}

	response := SpaceGetFieldResponse{
		Value: value,
	}

	err = sendMsg(conn, CommandNil, &response)
	if err != nil {
		log.WithError(err).Error("Failed to send response")
		return err
	}

	return nil
}
