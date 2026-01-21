package agent_client

import (
	"strings"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/paularlott/knot/internal/log"
)

func (c *AgentClient) initLogMessages() {
	log.Debug("initializing log message transport")

	for {
		logMsg := <-c.logChannel
		if logMsg == nil {
			continue
		}

		c.serverListMutex.RLock()
		for _, server := range c.serverList {
			select {
			case server.logChannel <- logMsg:
			default:
				// Queue full, drop message
			}
		}
		c.serverListMutex.RUnlock()
	}
}

func (c *AgentClient) SendLogMessage(service string, level msg.LogLevel, message string) error {
	// replace all \n without a \r with \r\n
	message = strings.ReplaceAll(message, "\n", "\r\n")

	// Strip any trailing \r\n
	message = strings.TrimRight(message, "\r\n")

	select {
	case c.logChannel <- &msg.LogMessage{
		Service: service,
		Level:   level,
		Message: message,
		Date:    time.Now(),
	}:
	default:
		// Queue full, drop message
	}

	return nil
}
