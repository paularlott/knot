package msg

import (
	"net"
	"time"

	"github.com/paularlott/knot/internal/log"
)

type LogLevel byte

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelError
)

type LogMessage struct {
	Level   LogLevel
	Service string
	Message string
	Date    time.Time
}

func SendLogMessage(conn net.Conn, message *LogMessage) error {
	logger := log.WithGroup("agent")
	// Write the command
	err := WriteCommand(conn, CmdLogMessage)
	if err != nil {
		logger.WithError(err).Error("writing state command")
		return err
	}

	// Write the message
	err = WriteMessage(conn, message)
	if err != nil {
		logger.WithError(err).Error("writing state message")
		return err
	}

	return nil
}
