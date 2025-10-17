package msg

import (
	"net"

	"github.com/paularlott/knot/internal/log"
)

type CreateTokenResponse struct {
	Server string
	Token  string
}

func SendRequestToken(conn net.Conn) (string, string, error) {
	logger := log.WithGroup("agent")
	// Write the state command
	err := WriteCommand(conn, CmdCreateToken)
	if err != nil {
		logger.WithError(err).Error("writing create token command")
		return "", "", err
	}

	// Read the response
	var response CreateTokenResponse
	err = ReadMessage(conn, &response)
	if err != nil {
		logger.WithError(err).Error("reading create token response")
		return "", "", err
	}

	return response.Server, response.Token, nil
}
