package msg

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

// UpdateRestrictionsMessage tells the agent what the space's restrictions
// ask of it: Locked locks the space's processes out of everything but file
// storage and personal secrets (knot restrict --lock), so the agent refuses
// their other requests, apart from restarting or shutting the space down. Sent from the server to the agent
// on every change; the registration response carries the same on
// (re)connect.
type UpdateRestrictionsMessage struct {
	Locked bool `json:"locked" msgpack:"locked"`
}

// SendUpdateRestrictions pushes the space's restrictions to the agent.
// Fire-and-forget, like SendUpdateJobs.
func SendUpdateRestrictions(conn net.Conn, locked bool) error {
	logger := log.WithGroup("agent")
	if err := WriteCommand(conn, CmdUpdateRestrictions); err != nil {
		logger.WithError(err).Error("writing update restrictions command")
		return err
	}
	if err := WriteMessage(conn, &UpdateRestrictionsMessage{Locked: locked}); err != nil {
		logger.WithError(err).Error("writing update restrictions message")
		return err
	}
	return nil
}
