package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/portforward"
)

func handleThrottlePort(conn net.Conn, msg *CommandMsg) {
	var request ThrottlePortRequest
	err := msg.Unmarshal(&request)
	if err != nil {
		log.WithError(err).Error("Failed to unmarshal throttle port request")
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: err.Error()})
		return
	}

	fwd, ok := portforward.GetForward(request.LocalPort)
	if !ok {
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "Port forward not found"})
		return
	}

	if request.Reset {
		fwd.SetThrottle(0, 0, 0)
	} else {
		fwd.SetThrottle(request.LatencyMs, request.JitterMs, request.BandwidthKB)
	}

	log.Info("port forward throttled", "local_port", request.LocalPort, "latency_ms", request.LatencyMs, "jitter_ms", request.JitterMs, "bandwidth_kb", request.BandwidthKB, "reset", request.Reset)
	sendMsg(conn, CommandNil, RunCommandResponse{Success: true})
}
