package apiv1

import (
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/api/agentv1"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/rest"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type AgentRegisterResponse struct {
  Status bool `json:"status"`
  AccessToken string `json:"access_token"`
}

func HandleRegisterAgent(w http.ResponseWriter, r *http.Request) {
  spaceId := chi.URLParam(r, "space_id")

  log.Debug().Msgf("agent registering for space %s", spaceId)

  // Test if an agent is registered for the space, in RegisteredAgents map
  database.AgentStateLock()
  if state, ok := database.AgentStateGet(spaceId); ok {
    log.Debug().Msgf("agent already registered for space %s", spaceId)

    // Load the space from the database
    db := database.GetInstance()
    space, err := db.GetSpace(spaceId)
    if err != nil {
      rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "space not found"})
      database.AgentStateUnlock()
      return
    }

    // Ping the existing agent to see if it's alive
    client := rest.NewClient(util.ResolveSRVHttp(space.GetAgentURL(), viper.GetString("server.namespace")), state.AccessToken)
    if agentv1.CallAgentPing(client) {
      log.Debug().Msgf("agent already registered for space %s and is alive", spaceId)
      rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "agent already registered for space"})
      database.AgentStateUnlock()
      return
    }
  }

  id, err := uuid.NewV7()
  if err != nil {
    log.Fatal().Msg(err.Error())
  }

  var token = id.String()
  database.AgentStateSet(spaceId, &database.AgentState{
    AccessToken: token,
    HasCodeServer: false,
    SSHPort: 0,
    HasTerminal: false,
    LastSeen: time.Now().UTC(),
  })
  database.AgentStateUnlock()

  response := AgentRegisterResponse{Status: true, AccessToken: token}
  rest.SendJSON(http.StatusOK, w, response)
}

func CallRegisterAgent(client *rest.RESTClient, spaceId string) (*AgentRegisterResponse, int, error) {
  response := &AgentRegisterResponse{}
  statusCode, err := client.Post(fmt.Sprintf("/api/v1/agents/%s", spaceId), nil, response, http.StatusOK)
  return response, statusCode, err
}

type AgentStatusRequest struct {
  HasCodeServer bool `json:"has_code_server"`
  SSHPort int `json:"ssh_port"`
  HasTerminal bool `json:"has_terminal"`
  TcpPorts []int `json:"tcp_ports"`
  HttpPorts []int `json:"http_ports"`
}

type AgentStatusResponse struct {
  Status bool `json:"status"`
}

func HandleAgentStatus(w http.ResponseWriter, r *http.Request) {
  spaceId := chi.URLParam(r, "space_id")

  request := AgentStatusRequest{}

  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Test if an agent is registered for the space, in RegisteredAgents map
  database.AgentStateLock()
  if state, ok := database.AgentStateGet(spaceId); ok {
    state.LastSeen = time.Now().UTC()
    state.HasCodeServer = request.HasCodeServer
    state.SSHPort = request.SSHPort
    state.HasTerminal = request.HasTerminal
    state.TcpPorts = request.TcpPorts
    state.HttpPorts = request.HttpPorts

    database.AgentStateUnlock()

    response := AgentStatusResponse{Status: true}
    rest.SendJSON(http.StatusOK, w, response)
    return
  }

  database.AgentStateUnlock()

  log.Debug().Msgf("agent status for space %s not found", spaceId)
  rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "agent not found"})
}

func CallUpdateAgentStatus(client *rest.RESTClient, spaceId string, hasCodeServer bool, sshPort int, hasTerminal bool, tcpPorts []int, httpPorts []int) (int, error) {
  request := &AgentStatusRequest{
    HasCodeServer: hasCodeServer,
    SSHPort: sshPort,
    HasTerminal: hasTerminal,
    TcpPorts: tcpPorts,
    HttpPorts: httpPorts,
  }
  response := &AgentStatusResponse{}
  statusCode, err := client.Post(fmt.Sprintf("/api/v1/agents/%s/status", spaceId), request, response, http.StatusOK)
  return statusCode, err
}
