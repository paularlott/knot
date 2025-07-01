package msg

import (
	"net"

	"github.com/rs/zerolog/log"
)

type SpaceNote struct {
	Note string
}

func SendSpaceNote(conn net.Conn, note string) error {
	// Write the state command
	err := WriteCommand(conn, CmdUpdateSpaceNote)
	if err != nil {
		log.Error().Msgf("agent: writing update note command: %v", err)
		return err
	}

	// Write the state message
	err = WriteMessage(conn, &SpaceNote{
		Note: note,
	})
	if err != nil {
		log.Error().Msgf("agent: writing update note message: %v", err)
		return err
	}

	return nil
}

func SendSpaceStop(conn net.Conn) error {
	// Write the state command
	err := WriteCommand(conn, CmdSpaceStop)
	if err != nil {
		log.Error().Msgf("agent: writing stop command: %v", err)
		return err
	}

	return nil
}

func SendSpaceRestart(conn net.Conn) error {
	// Write the state command
	err := WriteCommand(conn, CmdSpaceRestart)
	if err != nil {
		log.Error().Msgf("agent: writing restart command: %v", err)
		return err
	}

	return nil
}
