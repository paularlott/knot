package agent_client

import (
	"errors"
	"fmt"
	"net"
)

// AskServers opens a stream to each connected server in turn and passes it
// to ask, until ask has an answer: done, with the answer's error if it's a
// refusal, which is final rather than retried on another server. A server
// ask couldn't get an answer from is skipped, and if none answers, the
// errors are returned together.
func (c *AgentClient) AskServers(ask func(stream net.Conn) (done bool, err error)) error {
	c.serverListMutex.RLock()
	defer c.serverListMutex.RUnlock()

	var errs []error
	for _, server := range c.serverList {
		if server.muxSession == nil || server.muxSession.IsClosed() {
			continue
		}

		stream, err := server.muxSession.Open()
		if err != nil {
			errs = append(errs, fmt.Errorf("open stream to %v: %w", server, err))
			continue
		}
		done, err := ask(stream)
		stream.Close()
		if done {
			return err
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("request to %v: %w", server, err))
		}
	}

	if len(errs) == 0 {
		return errors.New("not connected to a knot server")
	}
	return errors.Join(errs...)
}
