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
	// Write the command
	err := WriteCommand(conn, CmdLogMessage)
	if err != nil {
		log.WithError(err).Error("agent: writing state command:")
		return err
	}

	// Write the message
	err = WriteMessage(conn, message)
	if err != nil {
		log.WithError(err).Error("agent: writing state message:")
		return err
	}

	return nil
}
