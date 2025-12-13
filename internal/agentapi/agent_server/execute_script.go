package agent_server

import (
	"github.com/paularlott/knot/internal/agentapi/msg"
)

func (s *Session) SendExecuteScript(execMsg *msg.ExecuteScriptMessage) (chan *msg.ExecuteScriptResponse, error) {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return nil, err
	}

	responseChannel := make(chan *msg.ExecuteScriptResponse, 1)

	go func() {
		defer conn.Close()
		defer close(responseChannel)

		err = msg.WriteCommand(conn, msg.CmdExecuteScript)
		if err != nil {
			s.logger.WithError(err).Error("writing execute script command:")
			responseChannel <- &msg.ExecuteScriptResponse{
				Success: false,
				Error:   "Failed to send command to agent",
			}
			return
		}

		err = msg.WriteMessage(conn, execMsg)
		if err != nil {
			s.logger.WithError(err).Error("writing execute script message:")
			responseChannel <- &msg.ExecuteScriptResponse{
				Success: false,
				Error:   "Failed to send script to agent",
			}
			return
		}

		var response msg.ExecuteScriptResponse
		err = msg.ReadMessage(conn, &response)
		if err != nil {
			s.logger.WithError(err).Error("reading execute script response:")
			responseChannel <- &msg.ExecuteScriptResponse{
				Success: false,
				Error:   "Failed to read response from agent",
			}
			return
		}

		responseChannel <- &response
	}()

	return responseChannel, nil
}
