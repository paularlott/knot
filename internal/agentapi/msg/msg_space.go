package msg

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

type SpaceNote struct {
	Note string
}

func SendSpaceNote(conn net.Conn, note string) error {
	logger := log.WithGroup("agent")
	// Write the state command
	err := WriteCommand(conn, CmdUpdateSpaceNote)
	if err != nil {
		logger.WithError(err).Error("writing update note command")
		return err
	}

	// Write the state message
	err = WriteMessage(conn, &SpaceNote{
		Note: note,
	})
	if err != nil {
		logger.WithError(err).Error("writing update note message")
		return err
	}

	return nil
}

func SendSpaceStop(conn net.Conn) error {
	logger := log.WithGroup("agent")
	// Write the state command
	err := WriteCommand(conn, CmdSpaceStop)
	if err != nil {
		logger.WithError(err).Error("writing stop command")
		return err
	}

	return nil
}

func SendSpaceRestart(conn net.Conn) error {
	logger := log.WithGroup("agent")
	// Write the state command
	err := WriteCommand(conn, CmdSpaceRestart)
	if err != nil {
		logger.WithError(err).Error("writing restart command")
		return err
	}

	return nil
}
