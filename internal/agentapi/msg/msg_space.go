package msg

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

type SpaceNote struct {
	Note string
}

func SendSpaceNote(conn net.Conn, note string) error {
	// Write the state command
	err := WriteCommand(conn, CmdUpdateSpaceNote)
	if err != nil {
		log.WithError(err).Error("agent: writing update note command:")
		return err
	}

	// Write the state message
	err = WriteMessage(conn, &SpaceNote{
		Note: note,
	})
	if err != nil {
		log.WithError(err).Error("agent: writing update note message:")
		return err
	}

	return nil
}

func SendSpaceStop(conn net.Conn) error {
	// Write the state command
	err := WriteCommand(conn, CmdSpaceStop)
	if err != nil {
		log.WithError(err).Error("agent: writing stop command:")
		return err
	}

	return nil
}

func SendSpaceRestart(conn net.Conn) error {
	// Write the state command
	err := WriteCommand(conn, CmdSpaceRestart)
	if err != nil {
		log.WithError(err).Error("agent: writing restart command:")
		return err
	}

	return nil
}
