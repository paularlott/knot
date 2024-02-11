package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
  AGENT_STATE_PING_INTERVAL = 4 * time.Second
  AGENT_STATE_GC_INTERVAL = 5 * time.Second
  AGENT_STATE_TIMEOUT = 15 * time.Second
)

// Struct holding the state of an agent
type AgentState struct {
	Id string `json:"space_id"`
  AccessToken string `json:"access_token"`
  HasCodeServer bool `json:"has_code_server"`
  SSHPort int `json:"ssh_port"`
  VNCHttpPort int `json:"vnc_http_port"`
  HasTerminal bool `json:"has_terminal"`
  TcpPorts []int `json:"tcp_ports"`
  HttpPorts []int `json:"http_ports"`
  ExpiresAfter time.Time `json:"expires_after"`
}

func NewAgentState(spaceId string) *AgentState {
  id, err := uuid.NewV7()
  if err != nil {
    log.Fatal().Msg(err.Error())
  }

  state := &AgentState{
    Id: spaceId,
    AccessToken: id.String(),
    HasCodeServer: false,
    SSHPort: 0,
    VNCHttpPort: 0,
    HasTerminal: false,
    TcpPorts: make([]int, 0),
    HttpPorts: make([]int, 0),
    ExpiresAfter: time.Now().UTC().Add(AGENT_STATE_TIMEOUT),
  }

  return state
}
