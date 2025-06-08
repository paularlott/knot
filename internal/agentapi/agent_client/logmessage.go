package agent_client

import (
	"strings"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/rs/zerolog/log"
)

func (c *AgentClient) initLogMessages() {
	log.Debug().Msg("agent: initializing log message transport")

	var err error

	for {
		logMsg := <-c.logChannel
		if logMsg == nil {
			continue
		}

		c.serverListMutex.RLock()
		for _, server := range c.serverList {
			if server.muxSession != nil && !server.muxSession.IsClosed() {
				if server.logConn == nil {
					log.Debug().Msgf("agent: opening logging connection to %s", server.address)

					server.logConn, err = server.muxSession.Open()
					if err != nil {
						log.Error().Err(err).Msgf("agent: failed to open mux session for server %s", server.address)
						continue
					}

					// Send any buffered messages
					for len(server.agentClient.logTempBuffer) > 0 {
						err := msg.SendLogMessage(server.logConn, server.agentClient.logTempBuffer[0])
						if err != nil {
							server.logConn.Close()
							server.logConn = nil
							break
						}

						// Remove the message from the buffer
						server.agentClient.logTempBuffer = server.agentClient.logTempBuffer[1:]
					}
				}
			}

			if server.logConn != nil {
				err := msg.SendLogMessage(server.logConn, logMsg)
				if err != nil {
					log.Error().Err(err).Msgf("agent: failed to send log message to server %s", server.address)
					server.agentClient.logTempBuffer = append(server.agentClient.logTempBuffer, logMsg)
					server.logConn.Close()
					server.logConn = nil
					break
				}
			} else {
				server.agentClient.logTempBuffer = append(server.agentClient.logTempBuffer, logMsg)
			}
		}
		c.serverListMutex.RUnlock()
	}
}

func (c *AgentClient) SendLogMessage(service string, level msg.LogLevel, message string) error {

	// If there are too many messages in the channel, discard the oldest one
	if len(c.logChannel) >= logChannelBufferSize {
		<-c.logChannel
	}

	// replace all \n without a \r with \r\n
	message = strings.ReplaceAll(message, "\n", "\r\n")

	// Strip any trailing \r\n
	message = strings.TrimRight(message, "\r\n")

	c.logChannel <- &msg.LogMessage{
		Service: service,
		Level:   level,
		Message: message,
		Date:    time.Now().UTC(),
	}

	return nil
}
