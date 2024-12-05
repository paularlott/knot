package agent_client

import (
	"net"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/rs/zerolog/log"
)

var logChannel chan *msg.LogMessage

func initLogMessages() {

	// If logChannel is already initialized, return
	if logChannel != nil {
		return
	}

	log.Debug().Msg("agent: initializing log message transport")

	logChannel = make(chan *msg.LogMessage, 100)

	go func() {
		var conn net.Conn
		var err error
		var tempBuffer []*msg.LogMessage

		for {
			// connect
			conn, err = muxSession.Open()
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}

			// Send any buffered messages
			for len(tempBuffer) > 0 {
				err := msg.SendLogMessage(conn, tempBuffer[0])
				if err != nil {
					conn.Close()
					conn = nil
					break
				}

				// Remove the message from the buffer
				tempBuffer = tempBuffer[1:]
			}

			if conn != nil {
				for {
					logMsg := <-logChannel
					if logMsg != nil {
						err := msg.SendLogMessage(conn, logMsg)
						if err != nil {
							tempBuffer = append(tempBuffer, logMsg)
							conn.Close()
							break
						}
					}
				}
			}

			time.Sleep(1 * time.Second)
		}
	}()
}

func SendLogMessage(msgType byte, message string) error {
	// replace all \n without a \r with \r\n
	message = strings.ReplaceAll(message, "\n", "\r\n")

	// Strip any trailing \r\n
	message = strings.TrimRight(message, "\r\n")

	logChannel <- &msg.LogMessage{
		MsgType: msgType,
		Message: message,
		Date:    time.Now().UTC(),
	}

	return nil
}
