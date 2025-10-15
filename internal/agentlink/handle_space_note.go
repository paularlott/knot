package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

func handleSpaceNote(conn net.Conn, msg *CommandMsg) error {
	var note SpaceNoteRequest
	err := msg.Unmarshal(&note)
	if err != nil {
		log.WithError(err).Error("agent: Failed to unmarshal space note")
		return err
	}

	err = agentClient.SendSpaceNote(note.Note)
	if err != nil {
		log.WithError(err).Error("agent: Failed to send space note")
		return err
	}

	return nil
}
