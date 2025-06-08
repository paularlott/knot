package agent_client

import (
	"errors"
	"fmt"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

func (c *AgentClient) SendRequestToken() (string, string, error) {
	c.serverListMutex.RLock()
	defer c.serverListMutex.RUnlock()

	var errs []error
	var host string
	var token string

	for _, server := range c.serverList {
		if server.muxSession != nil && !server.muxSession.IsClosed() {
			conn, err := server.muxSession.Open()
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to open mux session for server %v: %w", server, err))
				continue
			}

			host, token, err = msg.SendRequestToken(conn)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to request token from server %v: %w", server, err))
			}

			conn.Close()

			// If we've sent to one server then we can stop
			if err == nil {
				break
			}
		}
	}

	return host, token, errors.Join(errs...)
}
