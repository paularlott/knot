package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/portforward"
)

func handleStopPortForward(conn net.Conn, msg *CommandMsg) {
	var request StopPortForwardRequest
	err := msg.Unmarshal(&request)
	if err != nil {
		log.WithError(err).Error("Failed to unmarshal stop port forward request")
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: err.Error()})
		return
	}

	// Check if port forward exists
	if _, exists := portforward.GetForward(request.LocalPort); !exists {
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "port forward not found"})
		return
	}

	portforward.StopForward(request.LocalPort)
	sendMsg(conn, CommandNil, RunCommandResponse{Success: true})
}
