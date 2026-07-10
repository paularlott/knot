package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/agenttunnel"
	"github.com/paularlott/knot/internal/log"
)

func handleStopTunnel(conn net.Conn, msg *CommandMsg) {
	var request StopTunnelRequest
	if err := msg.Unmarshal(&request); err != nil {
		log.WithError(err).Error("Failed to unmarshal stop tunnel request")
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: err.Error()})
		return
	}

	if _, exists := agenttunnel.Get(request.Name); !exists {
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "tunnel not found"})
		return
	}

	agenttunnel.Stop(request.Name)

	sendMsg(conn, CommandNil, RunCommandResponse{Success: true})
}
