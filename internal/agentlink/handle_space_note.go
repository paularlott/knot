package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/agentapi/agent_client"

	"github.com/rs/zerolog/log"
)

func handleSpaceNote(conn net.Conn, msg *CommandMsg) error {
	var note SpaceNoteRequest
	err := msg.Unmarshal(&note)
	if err != nil {
		log.Error().Err(err).Msg("agent: Failed to unmarshal space note")
		return err
	}

	err = agent_client.SendSpaceNote(note.Note)
	if err != nil {
		log.Error().Err(err).Msg("agent: Failed to send space note")
		return err
	}

	return nil
}
