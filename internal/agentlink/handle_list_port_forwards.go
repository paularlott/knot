package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/portforward"
)

func handleListPortForwards(conn net.Conn, msg *CommandMsg) {
	forwards := portforward.ListForwards()

	response := ListPortForwardsResponse{
		Forwards: make([]PortForwardInfo, len(forwards)),
	}
	for i, fwd := range forwards {
		response.Forwards[i] = PortForwardInfo{
			LocalPort:  fwd.LocalPort,
			Space:      fwd.Space,
			RemotePort: fwd.RemotePort,
		}
	}

	err := sendMsg(conn, CommandNil, response)
	if err != nil {
		log.WithError(err).Error("Failed to send list response")
	}
}
