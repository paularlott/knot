package agent_client

import (
	"errors"
	"fmt"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

func (c *AgentClient) SendSpaceNote(note string) error {
	c.serverListMutex.RLock()
	defer c.serverListMutex.RUnlock()

	var errs []error

	for _, server := range c.serverList {
		if server.muxSession != nil && !server.muxSession.IsClosed() {
			conn, err := server.muxSession.Open()
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to open mux session for server %v: %w", server, err))
				continue
			}

			err = msg.SendSpaceNote(conn, note)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to send space note to server %v: %w", server, err))
			}

			conn.Close()

			// If we've sent to one server then we can stop
			if err == nil {
				break
			}
		}
	}

	return errors.Join(errs...)
}

func (c *AgentClient) SendSpaceVar(name, value string) error {
	c.serverListMutex.RLock()
	defer c.serverListMutex.RUnlock()

	var errs []error

	for _, server := range c.serverList {
		if server.muxSession != nil && !server.muxSession.IsClosed() {
			conn, err := server.muxSession.Open()
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to open mux session for server %v: %w", server, err))
				continue
			}

			err = msg.SendSpaceVar(conn, name, value)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to send space var to server %v: %w", server, err))
			}

			conn.Close()

			// If we've sent to one server then we can stop
			if err == nil {
				break
			}
		}
	}

	return errors.Join(errs...)
}

func (c *AgentClient) SendSpaceStop() error {
	c.serverListMutex.RLock()
	defer c.serverListMutex.RUnlock()

	var errs []error

	for _, server := range c.serverList {
		if server.muxSession != nil && !server.muxSession.IsClosed() {
			conn, err := server.muxSession.Open()
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to open mux session for server %v: %w", server, err))
				continue
			}

			err = msg.SendSpaceStop(conn)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to send space stop to server %v: %w", server, err))
			}

			conn.Close()

			// If we've sent to one server then we can stop
			if err == nil {
				break
			}
		}
	}

	return errors.Join(errs...)
}

func (c *AgentClient) SendSpaceRestart() error {
	c.serverListMutex.RLock()
	defer c.serverListMutex.RUnlock()

	var errs []error

	for _, server := range c.serverList {
		if server.muxSession != nil && !server.muxSession.IsClosed() {
			conn, err := server.muxSession.Open()
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to open mux session for server %v: %w", server, err))
				continue
			}

			err = msg.SendSpaceRestart(conn)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to send space restart to server %v: %w", server, err))
			}

			conn.Close()

			// If we've sent to one server then we can stop
			if err == nil {
				break
			}
		}
	}

	return errors.Join(errs...)
}
