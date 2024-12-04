package msg

import (
	"net"
	"strings"
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
	Source  byte
	Message string
	Date    time.Time
}

func SendLogMessage(conn net.Conn, source byte, message string) error {
	// Write the state command
	err := WriteCommand(conn, MSG_LOG_MSG)
	if err != nil {
		log.Error().Msgf("agent: writing state command: %v", err)
		return err
	}

	// replace all \n without a \r with \r\n
	message = strings.ReplaceAll(message, "\n", "\r\n")

	// Strip any trailing \r\n
	message = strings.TrimRight(message, "\r\n")

	// Write the state message
	err = WriteMessage(conn, &LogMessage{
		Source:  source,
		Message: message,
		Date:    time.Now().UTC(),
	})
	if err != nil {
		log.Error().Msgf("agent: writing state message: %v", err)
		return err
	}

	return nil
}
