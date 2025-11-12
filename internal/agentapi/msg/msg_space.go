package msg

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

type SpaceNote struct {
	Note string
}

type SpaceVar struct {
	Name  string
	Value string
}

type SpaceGetVar struct {
	Name string
}

type SpaceGetVarResponse struct {
	Value string
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

func SendSpaceVar(conn net.Conn, name, value string) error {
	logger := log.WithGroup("agent")
	// Write the command
	err := WriteCommand(conn, CmdUpdateSpaceVar)
	if err != nil {
		logger.WithError(err).Error("writing update var command")
		return err
	}

	// Write the message
	err = WriteMessage(conn, &SpaceVar{
		Name:  name,
		Value: value,
	})
	if err != nil {
		logger.WithError(err).Error("writing update var message")
		return err
	}

	return nil
}

func SendSpaceGetVar(conn net.Conn, name string) (string, error) {
	logger := log.WithGroup("agent")
	// Write the command
	err := WriteCommand(conn, CmdGetSpaceVar)
	if err != nil {
		logger.WithError(err).Error("writing get var command")
		return "", err
	}

	// Write the message
	err = WriteMessage(conn, &SpaceGetVar{
		Name: name,
	})
	if err != nil {
		logger.WithError(err).Error("writing get var message")
		return "", err
	}

	// Read the response
	var response SpaceGetVarResponse
	err = ReadMessage(conn, &response)
	if err != nil {
		logger.WithError(err).Error("reading get var response")
		return "", err
	}

	return response.Value, nil
}
