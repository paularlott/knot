package msg

import (
	"net"
	"time"

	"github.com/rs/zerolog/log"
)

type LogService byte
type LogLevel byte

const (
	ServiceSyslog LogService = iota
	ServiceAgent
)

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelError
)

type LogMessage struct {
	Service LogService
	Level   LogLevel
	Message string
	Date    time.Time
}

func SendLogMessage(conn net.Conn, message *LogMessage) error {
	// Write the command
	err := WriteCommand(conn, CmdLogMessage)
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
