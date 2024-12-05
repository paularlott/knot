package msg

import (
	"net"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	MSG_LOG_SYSLOG = iota
	MSG_LOG_DBG
	MSG_LOG_INF
	MSG_LOG_ERR
)

type LogMessage struct {
	MsgType byte
	Message string
	Date    time.Time
}

func SendLogMessage(conn net.Conn, message *LogMessage) error {
	// Write the command
	err := WriteCommand(conn, MSG_LOG_MSG)
	if err != nil {
		log.Error().Msgf("agent: writing state command: %v", err)
		return err
	}

	// Write the message
	err = WriteMessage(conn, message)
	if err != nil {
		log.Error().Msgf("agent: writing state message: %v", err)
		return err
	}

	return nil
}
