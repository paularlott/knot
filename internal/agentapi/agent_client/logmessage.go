package agent_client

import "github.com/paularlott/knot/internal/agentapi/msg"

func SendLogMessage(source byte, message string) error {

	// Open a connections over the mux session and write command
	if muxSession != nil {
		conn, err := muxSession.Open()
		if err != nil {
			return err
		}
		defer conn.Close()

		return msg.SendLogMessage(conn, source, message)
	}

	return nil
}
