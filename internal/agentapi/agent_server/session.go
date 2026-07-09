package agent_server

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/hashicorp/yamux"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
)

// Struct holding the state / registration information of an agent
type Session struct {
	Id                    string
	Version               string
	HasCodeServer         bool
	SSHPort               int
	VNCHttpPort           int
	HasTerminal           bool
	TcpPorts              map[string]string
	HttpPorts             map[string]string
	HasVSCodeTunnel       bool
	VSCodeTunnelName      string
	CPUPercent            float64
	MemoryUsedBytes       uint64
	MemoryLimitBytes      uint64
	DiskUsedBytes         uint64
	DiskLimitBytes        uint64
	ActivityWriteCount    uint32
	ActivityCreateCount   uint32
	ActivityDeleteCount   uint32
	ActivityRenameCount   uint32
	ActivityDistinctPaths uint32
	LastActivityAtUnix    int64
	MethodCallsTotal      uint64
	HTTPRequestsTotal     uint64
	TCPConnectionsTotal   uint64
	MethodRPS             float64
	HTTPRPS               float64
	TCPRPS                float64
	LastStateAt           time.Time
	lastStateAtMu         sync.Mutex
	LastPingAt            time.Time
	lastPingAtMu          sync.Mutex
	MuxSession            *yamux.Session
	logger                logger.Logger

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
		LastStateAt:       time.Now().UTC(),
		LastPingAt:        time.Now().UTC(),
		MuxSession:        nil,
		LogHistoryMutex:   &sync.RWMutex{},
		LogHistory:        make([]*msg.LogMessage, 0),
		LogListenersMutex: &sync.RWMutex{},
		LogListeners:      make(map[string]chan *msg.LogMessage),
	}
}

// GetLastStateAt returns the time of the last state report. LastStateAt is
// written by the agent state handler and the stale-session checker from
// different goroutines, so all access goes through these helpers.
func (s *Session) GetLastStateAt() time.Time {
	s.lastStateAtMu.Lock()
	defer s.lastStateAtMu.Unlock()
	return s.LastStateAt
}

// SetLastStateAt records the time of the last state report.
func (s *Session) SetLastStateAt(t time.Time) {
	s.lastStateAtMu.Lock()
	defer s.lastStateAtMu.Unlock()
	s.LastStateAt = t
}

// GetLastPingAt returns the time of the last successful mux ping.
func (s *Session) GetLastPingAt() time.Time {
	s.lastPingAtMu.Lock()
	defer s.lastPingAtMu.Unlock()
	return s.LastPingAt
}

// SetLastPingAt records the time of the last successful mux ping.
func (s *Session) SetLastPingAt(t time.Time) {
	s.lastPingAtMu.Lock()
	defer s.lastPingAtMu.Unlock()
	s.LastPingAt = t
}

// TelemetryLive reports whether a real agent state report (CmdUpdateState) has
// been received within the agent liveness window. A successful mux ping does
// NOT count: the state-reporting loop and the ping responder are independent
// goroutines, so a wedged state loop can leave a ping-alive session holding a
// frozen last reading. Callers that want to present data as "current" (e.g. the
// usage gauge) must check this rather than just session presence.
func (s *Session) TelemetryLive() bool {
	last := s.GetLastStateAt()
	if last.IsZero() {
		return false
	}
	return time.Since(last) <= AGENT_LIVENESS_TIMEOUT
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

// CloseLogListeners closes and removes every registered log listener. Called
// when the session is torn down (e.g. the space terminated) so that streaming
// log readers unblock on their channel and close the client WebSocket — the
// same way the terminal closes when the mux session ends. Entries are deleted
// as they're closed so a concurrent UnregisterLogListener won't double-close.
func (s *Session) CloseLogListeners() {
	s.LogListenersMutex.Lock()
	defer s.LogListenersMutex.Unlock()

	for id, c := range s.LogListeners {
		close(c)
		delete(s.LogListeners, id)
	}
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

func (s *Session) SendUpdateAuthorizedKeys(sshKeys []string, sshPrivateKey string, githubUsernames []string) error {
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
		SSHPrivateKey:   sshPrivateKey,
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

func (s *Session) SendUpdateHealthConfig(config *msg.HealthConfig) error {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	err = msg.WriteCommand(conn, msg.CmdUpdateHealthConfig)
	if err != nil {
		s.logger.WithError(err).Error("writing update health config command:")
		return err
	}

	err = msg.WriteMessage(conn, config)
	if err != nil {
		s.logger.WithError(err).Error("writing update health config message:")
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

func (s *Session) SendPortForward(portCmd *msg.PortForwardRequest) (*msg.PortForwardResponse, error) {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Write the port forward command
	err = msg.WriteCommand(conn, msg.CmdPortForward)
	if err != nil {
		s.logger.WithError(err).Error("writing port forward command:")
		return &msg.PortForwardResponse{Success: false, Error: "Failed to send command to agent"}, nil
	}

	// Write the port forward message
	err = msg.WriteMessage(conn, portCmd)
	if err != nil {
		s.logger.WithError(err).Error("writing port forward message:")
		return &msg.PortForwardResponse{Success: false, Error: "Failed to send command message to agent"}, nil
	}

	// Read the response
	var response msg.PortForwardResponse
	err = msg.ReadMessage(conn, &response)
	if err != nil {
		s.logger.WithError(err).Error("reading port forward response:")
		return &msg.PortForwardResponse{Success: false, Error: "Failed to read response from agent"}, nil
	}

	return &response, nil
}

func (s *Session) SendPortList() (*msg.PortListResponse, error) {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Write the port list command
	err = msg.WriteCommand(conn, msg.CmdPortList)
	if err != nil {
		s.logger.WithError(err).Error("writing port list command:")
		return nil, err
	}

	// Read the response
	var response msg.PortListResponse
	err = msg.ReadMessage(conn, &response)
	if err != nil {
		s.logger.WithError(err).Error("reading port list response:")
		return nil, err
	}

	return &response, nil
}

func (s *Session) SendPortStop(portCmd *msg.PortStopRequest) (*msg.PortStopResponse, error) {
	conn, err := s.MuxSession.Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Write the port stop command
	err = msg.WriteCommand(conn, msg.CmdPortStop)
	if err != nil {
		s.logger.WithError(err).Error("writing port stop command:")
		return &msg.PortStopResponse{Success: false, Error: "Failed to send command to agent"}, nil
	}

	// Write the port stop message
	err = msg.WriteMessage(conn, portCmd)
	if err != nil {
		s.logger.WithError(err).Error("writing port stop message:")
		return &msg.PortStopResponse{Success: false, Error: "Failed to send command message to agent"}, nil
	}

	// Read the response
	var response msg.PortStopResponse
	err = msg.ReadMessage(conn, &response)
	if err != nil {
		s.logger.WithError(err).Error("reading port stop response:")
		return &msg.PortStopResponse{Success: false, Error: "Failed to read response from agent"}, nil
	}

	return &response, nil
}
