package msg

import (
	"net"

	"github.com/rs/zerolog/log"
)

type CreateTokenResponse struct {
	Server string
	Token  string
}

func SendRequestToken(conn net.Conn) (string, string, error) {
	// Write the state command
	err := WriteCommand(conn, CmdCreateToken)
	if err != nil {
		log.Error().Msgf("agent: writing create token command: %v", err)
		return "", "", err
	}

	// Read the response
	var response CreateTokenResponse
	err = ReadMessage(conn, &response)
	if err != nil {
		log.Error().Msgf("agent: reading create token response: %v", err)
		return "", "", err
	}

	return response.Server, response.Token, nil
}
