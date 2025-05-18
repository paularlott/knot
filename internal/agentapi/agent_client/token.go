package agent_client

import (
	"fmt"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

func SendRequestToken() (string, string, error) {
	if muxSession == nil {
		return "", "", fmt.Errorf("no mux session")
	}

	// connect
	conn, err := muxSession.Open()
	if err != nil {
		return "", "", err
	}
	defer conn.Close()

	return msg.SendRequestToken(conn)
}
