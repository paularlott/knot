package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

func handleRegisterMethods(conn net.Conn, msg *CommandMsg) {
	var req RegisterMethodsRequest
	if err := msg.Unmarshal(&req); err != nil {
		log.WithError(err).Error("failed to unmarshal register methods request")
		_ = sendMsg(conn, CommandRegisterMethods, RegisterMethodsResponse{Success: false, Error: err.Error()})
		return
	}

	if agentClient == nil {
		_ = sendMsg(conn, CommandRegisterMethods, RegisterMethodsResponse{Success: false, Error: "agent is not connected"})
		return
	}

	if err := agentClient.RegisterMethods(&req.Registration); err != nil {
		_ = sendMsg(conn, CommandRegisterMethods, RegisterMethodsResponse{Success: false, Error: err.Error()})
		return
	}

	_ = sendMsg(conn, CommandRegisterMethods, RegisterMethodsResponse{Success: true})
}
