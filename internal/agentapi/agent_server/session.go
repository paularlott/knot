package agent_server

import (
	"sync"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/hashicorp/yamux"
	"github.com/rs/zerolog/log"
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
	AgentIp          string
	MuxSession       *yamux.Session

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
		HasCodeServer:     false,
		SSHPort:           0,
		VNCHttpPort:       0,
		HasTerminal:       false,
		TcpPorts:          make(map[string]string, 0),
		HttpPorts:         make(map[string]string, 0),
		AgentIp:           "",
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
		log.Error().Msg(err.Error())
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
		log.Error().Msgf("agent: writing update authorized keys command: %v", err)
		return err
	}

	// Write the update authorized keys message
	err = msg.WriteMessage(conn, &msg.UpdateAuthorizedKeys{
		SSHKeys:         sshKeys,
		GitHubUsernames: githubUsernames,
	})
	if err != nil {
		log.Error().Msgf("agent: writing update authorized keys message: %v", err)
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
		log.Error().Msgf("agent: writing update shell command: %v", err)
		return err
	}

	// Write the update shell message
	err = msg.WriteMessage(conn, &msg.UpdateShell{
		Shell: shell,
	})
	if err != nil {
		log.Error().Msgf("agent: writing update shell message: %v", err)
		return err
	}

	return nil
}
