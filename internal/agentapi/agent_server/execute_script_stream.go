package agent_server

import (
	"io"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

func (s *Session) SendExecuteScriptStream(execMsg *msg.ExecuteScriptStreamMessage) (io.ReadWriteCloser, error) {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return nil, err
	}

	err = msg.WriteCommand(conn, msg.CmdExecuteScriptStream)
	if err != nil {
		s.logger.WithError(err).Error("writing execute script stream command")
		conn.Close()
		return nil, err
	}

	err = msg.WriteMessage(conn, execMsg)
	if err != nil {
		s.logger.WithError(err).Error("writing execute script stream message")
		conn.Close()
		return nil, err
	}

	return conn, nil
}
