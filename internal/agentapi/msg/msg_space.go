package msg

import (
	"net"

	"github.com/rs/zerolog/log"
)

type SpaceDescription struct {
	Description string
}

func SendSpaceDescription(conn net.Conn, description string) error {
	// Write the state command
	err := WriteCommand(conn, CmdUpdateSpaceDescription)
	if err != nil {
		log.Error().Msgf("agent: writing update description command: %v", err)
		return err
	}

	// Write the state message
	err = WriteMessage(conn, &SpaceDescription{
		Description: description,
	})
	if err != nil {
		log.Error().Msgf("agent: writing update description message: %v", err)
		return err
	}

	return nil
}
