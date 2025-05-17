package agent_client

import (
	"github.com/paularlott/knot/internal/agentapi/msg"
)

func SendSpaceNote(note string) error {

	// connect
	conn, err := muxSession.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	return msg.SendSpaceNote(conn, note)
}
