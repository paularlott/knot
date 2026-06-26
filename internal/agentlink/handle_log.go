package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/log"
)

// handleLog processes a stream of CommandLog messages on a single connection.
// The first message (already read by handleCommandConnection) is processed,
// then subsequent messages are read with no deadline until the peer closes the
// connection. Each line is forwarded upstream via the agent client's log
// channel, mirroring how remote/streaming script execution surfaces logs.
func handleLog(conn net.Conn, first *CommandMsg) {
	current := first
	for current != nil {
		var req LogRequest
		if err := current.Unmarshal(&req); err != nil {
			log.WithError(err).Error("failed to unmarshal log request")
		} else if agentClient != nil {
			_ = agentClient.SendLogMessage(req.Service, msg.LogLevel(req.Level), req.Message)
		}

		next, err := receiveMsgTimeout(conn, 0)
		if err != nil {
			return
		}
		current = next
	}
}
