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
		mode := fwd.GetMode()
		if mode == "" {
			mode = "relay"
		}
		latencyMs, jitterMs, bandwidthKB, timeoutMs, down := fwd.GetThrottle()
		response.Forwards[i] = PortForwardInfo{
			LocalPort:   fwd.LocalPort,
			Space:       fwd.Space,
			RemotePort:  fwd.RemotePort,
			Persistent:  portforward.IsPersistent(fwd.LocalPort),
			Mode:        mode,
			LatencyMs:   latencyMs,
			JitterMs:    jitterMs,
			BandwidthKB: bandwidthKB,
			TimeoutMs:   timeoutMs,
			Down:        down,
		}
	}

	err := sendMsg(conn, CommandNil, response)
	if err != nil {
		log.WithError(err).Error("Failed to send list response")
	}
}
