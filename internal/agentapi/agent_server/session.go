package agent_server

import (
	"sync"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/hashicorp/yamux"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
)

// Struct holding the state / registration information of an agent
type Session struct {
	Id               string
	Version          string
	HasCodeServer    bool
	SSHPort          int
	VNCHttpPort      int
	HasTerminal      bool
	TcpPorts         map[string]string
	HttpPorts        map[string]string
	HasVSCodeTunnel  bool
	VSCodeTunnelName string
	MuxSession       *yamux.Session
	logger           logger.Logger

	// The log history
	LogHistoryMutex *sync.RWMutex
	LogHistory      []*msg.LogMessage

	// The list of listeners for log messages
	LogListenersMutex *sync.RWMutex
	LogListeners      map[string]chan *msg.LogMessage
}

// creates a new agent session
func NewSession(spaceId string, version string) *Session {
	return &Session{
		Id:                spaceId,
		Version:           version,
		logger:            log.WithGroup("agent"),
		HasCodeServer:     false,
		SSHPort:           0,
		VNCHttpPort:       0,
		HasTerminal:       false,
		TcpPorts:          make(map[string]string, 0),
		HttpPorts:         make(map[string]string, 0),
		MuxSession:        nil,
		LogHistoryMutex:   &sync.RWMutex{},
		LogHistory:        make([]*msg.LogMessage, 0),
		LogListenersMutex: &sync.RWMutex{},
		LogListeners:      make(map[string]chan *msg.LogMessage),
	}
}

func (s *Session) RegisterLogListener() (string, chan *msg.LogMessage) {
	id, err := uuid.NewV7()
	if err != nil {
		s.logger.Error(err.Error())
		return "", nil
	}

	s.LogListenersMutex.Lock()
	defer s.LogListenersMutex.Unlock()

	s.LogListeners[id.String()] = make(chan *msg.LogMessage, 100)

	return id.String(), s.LogListeners[id.String()]
}

func (s *Session) UnregisterLogListener(listenerId string) {
	s.LogListenersMutex.Lock()
	defer s.LogListenersMutex.Unlock()

	if c, ok := s.LogListeners[listenerId]; ok {
		close(c)
	}

	delete(s.LogListeners, listenerId)
}

func (s *Session) Ping() bool {
	// Open a connections over the mux session and write a ping command
	conn, err := s.MuxSession.Open()
	if err != nil {
		return false
	}
	defer conn.Close()

	// Write the ping command
	err = msg.WriteCommand(conn, msg.CmdPing)
	if err != nil {
		return false
	}

	// Wait for the ping response
	var pong msg.Pong
	err = msg.ReadMessage(conn, &pong)
	if err != nil || pong.Payload != "pong" {
		return false
	}

	return true
}

func (s *Session) SendUpdateAuthorizedKeys(sshKeys []string, githubUsernames []string) error {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	// Write the update authorized keys command
	err = msg.WriteCommand(conn, msg.CmdUpdateAuthorizedKeys)
	if err != nil {
		s.logger.WithError(err).Error("writing update authorized keys command:")
		return err
	}

	// Write the update authorized keys message
	err = msg.WriteMessage(conn, &msg.UpdateAuthorizedKeys{
		SSHKeys:         sshKeys,
		GitHubUsernames: githubUsernames,
	})
	if err != nil {
		s.logger.WithError(err).Error("writing update authorized keys message:")
		return err
	}

	return nil
}

func (s *Session) SendUpdateShell(shell string) error {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	// Write the update shell command
	err = msg.WriteCommand(conn, msg.CmdUpdateShell)
	if err != nil {
		s.logger.WithError(err).Error("writing update shell command:")
		return err
	}

	// Write the update shell message
	err = msg.WriteMessage(conn, &msg.UpdateShell{
		Shell: shell,
	})
	if err != nil {
		s.logger.WithError(err).Error("writing update shell message:")
		return err
	}

	return nil
}

func (s *Session) SendRunCommand(runCmd *msg.RunCommandMessage) (chan *msg.RunCommandResponse, error) {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return nil, err
	}

	// Create a response channel
	responseChannel := make(chan *msg.RunCommandResponse, 1)

	// Handle the command in a goroutine
	go func() {
		defer conn.Close()
		defer close(responseChannel)

		// Write the run command
		err = msg.WriteCommand(conn, msg.CmdRunCommand)
		if err != nil {
			s.logger.WithError(err).Error("writing run command:")
			responseChannel <- &msg.RunCommandResponse{
				Success: false,
				Error:   "Failed to send command to agent",
			}
			return
		}

		// Write the run command message
		err = msg.WriteMessage(conn, runCmd)
		if err != nil {
			s.logger.WithError(err).Error("writing run command message:")
			responseChannel <- &msg.RunCommandResponse{
				Success: false,
				Error:   "Failed to send command message to agent",
			}
			return
		}

		// Read the response
		var response msg.RunCommandResponse
		err = msg.ReadMessage(conn, &response)
		if err != nil {
			s.logger.WithError(err).Error("reading run command response:")
			responseChannel <- &msg.RunCommandResponse{
				Success: false,
				Error:   "Failed to read response from agent",
			}
			return
		}

		responseChannel <- &response
	}()

	return responseChannel, nil
}

func (s *Session) SendCopyFile(copyCmd *msg.CopyFileMessage) (chan *msg.CopyFileResponse, error) {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return nil, err
	}

	// Create a response channel
	responseChannel := make(chan *msg.CopyFileResponse, 1)

	// Handle the command in a goroutine
	go func() {
		defer conn.Close()
		defer close(responseChannel)

		// Write the copy file command
		err = msg.WriteCommand(conn, msg.CmdCopyFile)
		if err != nil {
			s.logger.WithError(err).Error("writing copy file command:")
			responseChannel <- &msg.CopyFileResponse{
				Success: false,
				Error:   "Failed to send command to agent",
			}
			return
		}

		// Write the copy file message
		err = msg.WriteMessage(conn, copyCmd)
		if err != nil {
			s.logger.WithError(err).Error("writing copy file message:")
			responseChannel <- &msg.CopyFileResponse{
				Success: false,
				Error:   "Failed to send command message to agent",
			}
			return
		}

		// Read the response
		var response msg.CopyFileResponse
		err = msg.ReadMessage(conn, &response)
		if err != nil {
			s.logger.WithError(err).Error("reading copy file response:")
			responseChannel <- &msg.CopyFileResponse{
				Success: false,
				Error:   "Failed to read response from agent",
			}
			return
		}

		responseChannel <- &response
	}()

	return responseChannel, nil
}
