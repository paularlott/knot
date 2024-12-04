package agent_server

import (
	"sync"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/hashicorp/yamux"
	"github.com/rs/zerolog/log"
)

// Struct holding the state / registration information of an agent
type Session struct {
	Id               string                          `msgpack:"space_id"`
	Version          string                          `msgpack:"version"`
	HasCodeServer    bool                            `msgpack:"has_code_server"`
	SSHPort          int                             `msgpack:"ssh_port"`
	VNCHttpPort      int                             `msgpack:"vnc_http_port"`
	HasTerminal      bool                            `msgpack:"has_terminal"`
	TcpPorts         map[string]string               `msgpack:"tcp_ports"`
	HttpPorts        map[string]string               `msgpack:"http_ports"`
	HasVSCodeTunnel  bool                            `msgpack:"has_vscode_tunnel"`
	VSCodeTunnelName string                          `msgpack:"vscode_tunnel_name"`
	AgentIp          string                          `msgpack:"agent_ip"`
	ExpiresAfter     time.Time                       `msgpack:"-"`
	MuxSession       *yamux.Session                  `msgpack:"-"`
	LogHistoryMutex  *sync.RWMutex                   `msgpack:"-"`
	LogHistory       []*msg.LogMessage               `msgpack:"-"`
	LogNotifyMutex   *sync.RWMutex                   `msgpack:"-"`
	LogNotify        map[string]chan *msg.LogMessage `msgpack:"-"`
}

// creates a new agent session
func NewSession(spaceId string, version string) *Session {
	return &Session{
		Id:              spaceId,
		Version:         version,
		HasCodeServer:   false,
		SSHPort:         0,
		VNCHttpPort:     0,
		HasTerminal:     false,
		TcpPorts:        make(map[string]string, 0),
		HttpPorts:       make(map[string]string, 0),
		ExpiresAfter:    time.Now().UTC().Add(AGENT_SESSION_TIMEOUT),
		AgentIp:         "",
		MuxSession:      nil,
		LogHistoryMutex: &sync.RWMutex{},
		LogHistory:      make([]*msg.LogMessage, 0),
		LogNotifyMutex:  &sync.RWMutex{},
		LogNotify:       make(map[string]chan *msg.LogMessage, 0),
	}
}

func (s *Session) RegisterLogSink(spaceId string) chan *msg.LogMessage {
	s.LogNotifyMutex.Lock()
	defer s.LogNotifyMutex.Unlock()

	// Check if the log sink is already registered
	if _, ok := s.LogNotify[spaceId]; ok {
		return nil
	}

	c := make(chan *msg.LogMessage, 100)
	s.LogNotify[spaceId] = c

	return c
}

func (s *Session) UnregisterLogSink(spaceId string) {
	s.LogNotifyMutex.Lock()
	defer s.LogNotifyMutex.Unlock()

	if c, ok := s.LogNotify[spaceId]; ok {
		close(c)
	}

	delete(s.LogNotify, spaceId)
}

func (s *Session) Ping() bool {
	// Open a connections over the mux session and write a ping command
	conn, err := s.MuxSession.Open()
	if err != nil {
		return false
	}
	defer conn.Close()

	// Write the ping command
	err = msg.WriteCommand(conn, msg.MSG_PING)
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

func (s *Session) SendUpdateAuthorizedKeys(sshKey string, githubUsername string) error {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	// Write the update authorized keys command
	err = msg.WriteCommand(conn, msg.MSG_UPDATE_AUTHORIZED_KEYS)
	if err != nil {
		log.Error().Msgf("agent: writing update authorized keys command: %v", err)
		return err
	}

	// Write the update authorized keys message
	err = msg.WriteMessage(conn, &msg.UpdateAuthorizedKeys{
		SSHKey:         sshKey,
		GitHubUsername: githubUsername,
	})
	if err != nil {
		log.Error().Msgf("agent: writing update authorized keys message: %v", err)
		return err
	}

	return nil
}
