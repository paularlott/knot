package agent_client

import (
	"github.com/paularlott/knot/internal/agentapi/msg"
)

func SendSpaceDescription(description string) error {

	// connect
	conn, err := muxSession.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	return msg.SendSpaceDescription(conn, description)
}
