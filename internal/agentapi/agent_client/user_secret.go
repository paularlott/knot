package agent_client

import (
	"errors"
	"fmt"
	"io"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

// ErrUserSecretsUnsupported is returned when no connected server answers
// personal secret requests: a server older than the feature closes the
// stream on the unknown command.
var ErrUserSecretsUnsupported = errors.New("personal secrets are not supported by this server")

// ResolveUserSecret reads one of the space owner's personal secrets from
// the server over the agent link. The first server that answers decides:
// a refusal (no token, template switched off, not found) is final rather
// than retried on another server.
func (c *AgentClient) ResolveUserSecret(path, field string) (string, error) {
	c.serverListMutex.RLock()
	defer c.serverListMutex.RUnlock()

	var errs []error
	for _, server := range c.serverList {
		if server.muxSession == nil || server.muxSession.IsClosed() {
			continue
		}

		conn, err := server.muxSession.Open()
		if err != nil {
			errs = append(errs, fmt.Errorf("open stream to %v: %w", server, err))
			continue
		}

		response, err := msg.SendResolveUserSecret(conn, path, field)
		conn.Close()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return "", ErrUserSecretsUnsupported
			}
			errs = append(errs, fmt.Errorf("request to %v: %w", server, err))
			continue
		}

		if response.Error != "" {
			return "", errors.New(response.Error)
		}
		return response.Value, nil
	}

	if len(errs) == 0 {
		return "", errors.New("not connected to a knot server")
	}
	return "", errors.Join(errs...)
}
