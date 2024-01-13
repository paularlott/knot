package database

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Struct holding the state of an agent
type AgentState struct {
  AccessToken string
  HasCodeServer bool
  SSHPort int
  HasTerminal bool
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
      time.Sleep(5 * time.Second) // TODO make this configurable or at least set a sane amount of time

      // Loop through all the agents and check if they haven't been seen in a while
      registeredAgentsMutex.Lock()
      for spaceId, agent := range registeredAgents {
        var lastSeen = time.Now().UTC().Sub(agent.LastSeen)
        if lastSeen > 15 * time.Second { // TODO make this configurable or at least set a sane amount of time 2 x agent ping interval
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
