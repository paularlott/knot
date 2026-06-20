package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

// handleUnregisterMethods removes all methods for the space by calling
// agentClient.UnregisterAllMethods, which sends CmdUnregisterMethods to the
// knot server, stops the stdio method server process, and clears the stashed
// registration so reconnect doesn't republish dead methods.
func handleUnregisterMethods(conn net.Conn, _ *CommandMsg) {
	if agentClient == nil {
		_ = sendMsg(conn, CommandUnregisterMethods, RegisterMethodsResponse{Success: false, Error: "agent is not connected"})
		return
	}

	if err := agentClient.UnregisterAllMethods(); err != nil {
		log.WithError(err).Warn("unregister methods failed")
		_ = sendMsg(conn, CommandUnregisterMethods, RegisterMethodsResponse{Success: false, Error: err.Error()})
		return
	}

	_ = sendMsg(conn, CommandUnregisterMethods, RegisterMethodsResponse{Success: true})
}
