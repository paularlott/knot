package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

func handleStopPortForward(conn net.Conn, msg *CommandMsg) {
	var request StopPortForwardRequest
	err := msg.Unmarshal(&request)
	if err != nil {
		log.WithError(err).Error("Failed to unmarshal stop port forward request")
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: err.Error()})
		return
	}

	portForwardsMux.Lock()
	fwd, exists := portForwards[request.LocalPort]
	if !exists {
		portForwardsMux.Unlock()
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "port forward not found"})
		return
	}

	// Cancel the forward and close the listener
	fwd.Cancel()
	if fwd.Listener != nil {
		fwd.Listener.Close()
	}
	delete(portForwards, request.LocalPort)
	portForwardsMux.Unlock()

	sendMsg(conn, CommandNil, RunCommandResponse{Success: true})
}
