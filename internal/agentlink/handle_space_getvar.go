package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

func handleSpaceGetVar(conn net.Conn, msg *CommandMsg) error {
	var varReq SpaceGetVarRequest
	err := msg.Unmarshal(&varReq)
	if err != nil {
		log.WithError(err).Error("Failed to unmarshal space get var")
		return err
	}

	value, err := agentClient.SendSpaceGetVar(varReq.Name)
	if err != nil {
		log.WithError(err).Error("Failed to get space var")
		return err
	}

	response := SpaceGetVarResponse{
		Value: value,
	}

	err = sendMsg(conn, CommandNil, &response)
	if err != nil {
		log.WithError(err).Error("Failed to send response")
		return err
	}

	return nil
}
