package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

func handleListPortForwards(conn net.Conn, msg *CommandMsg) {
	portForwardsMux.RLock()
	defer portForwardsMux.RUnlock()

	forwards := make([]PortForwardInfo, 0, len(portForwards))
	for _, fwd := range portForwards {
		forwards = append(forwards, PortForwardInfo{
			LocalPort:  fwd.LocalPort,
			Space:      fwd.Space,
			RemotePort: fwd.RemotePort,
		})
	}

	response := ListPortForwardsResponse{
		Forwards: forwards,
	}

	err := sendMsg(conn, CommandNil, response)
	if err != nil {
		log.WithError(err).Error("Failed to send list response")
	}
}
