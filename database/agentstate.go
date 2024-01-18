package database

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
  AGENT_STATE_PING_INTERVAL = 5 * time.Second
  AGENT_STATE_TIMEOUT = 15 * time.Second
)

// Struct holding the state of an agent
type AgentState struct {
  AccessToken string
  HasCodeServer bool
  SSHPort int
  HasTerminal bool
  TcpPorts []int
  HttpPorts []int
  LastSeen time.Time
}

var (
  registeredAgents map[string]*AgentState
  registeredAgentsMutex sync.Mutex
)

func InitializeAgentInformation() {
  registeredAgents = make(map[string]*AgentState)

  // Start a go routine to check for agents that haven't been seen in a while
  go func() {
    for {
      time.Sleep(AGENT_STATE_PING_INTERVAL)

      // Loop through all the agents and check if they haven't been seen in a while
      registeredAgentsMutex.Lock()
      for spaceId, agent := range registeredAgents {
        var lastSeen = time.Now().UTC().Sub(agent.LastSeen)
        if lastSeen > AGENT_STATE_TIMEOUT {
          log.Debug().Msgf("agent %s not seen for a while, dropping agent", spaceId)

          delete(registeredAgents, spaceId)
        }
      }
      registeredAgentsMutex.Unlock()
    }
  }()
}

func AgentStateLock() {
  registeredAgentsMutex.Lock()
}

func AgentStateUnlock() {
  registeredAgentsMutex.Unlock()
}

func AgentStateGet(spaceId string) (*AgentState, bool) {
  state, ok := registeredAgents[spaceId]
  return state, ok
}

func AgentStateSet(spaceId string, state *AgentState) {
  registeredAgents[spaceId] = state
}

func AgentStateRemove(spaceId string) {
  delete(registeredAgents, spaceId)
}
