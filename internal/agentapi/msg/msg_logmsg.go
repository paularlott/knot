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

	// Fields carries optional structured key/values from structured ingest
	// formats (e.g. VictoriaLogs jsonline) through to log sinks and space
	// log forwarding. Nil for unstructured sources.
	Fields map[string]string `msgpack:"fields"`
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
