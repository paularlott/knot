package apiv1

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/api/agentv1"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/rest"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type AgentRegisterResponse struct {
	Status         bool   `json:"status"`
	AccessToken    string `json:"access_token"`
	ServerURL      string `json:"server_url"`
	SSHKey         string `json:"ssh_key"`
	GitHubUsername string `json:"github_username"`
}

func HandleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	db := database.GetInstance()
	cache := database.GetCacheInstance()
	spaceId := chi.URLParam(r, "space_id")

	log.Debug().Msgf("agent registering for space %s", spaceId)

	// Test if an agent is registered for the space, in RegisteredAgents map
	if state, err := cache.GetAgentState(spaceId); err != nil {
		log.Debug().Msgf("agent already registered for space %s", spaceId)

		// Load the space from the database
		space, err := db.GetSpace(spaceId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "space not found"})
			return
		}

		// Ping the existing agent to see if it's alive
		client := rest.NewClient(util.ResolveSRVHttp(space.GetAgentURL()), state.AccessToken, viper.GetBool("tls_skip_verify"))
		if agentv1.CallAgentPing(client) {
			log.Debug().Msgf("agent already registered for space %s and is alive", spaceId)
			rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "agent already registered for space"})
			return
		}
	}

	state := model.NewAgentState(spaceId)
	cache.SaveAgentState(state)

	var serverURL string
	if viper.GetString("server.agent_url") != "" {
		serverURL = viper.GetString("server.agent_url")
	} else {
		serverURL = viper.GetString("server.url")
	}

	// Load the space from the database
	space, err := db.GetSpace(spaceId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "space not found"})
		return
	}

	// Load the user that owns the space
	user, err := db.GetUser(space.UserId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "user not found"})
		return
	}

	response := AgentRegisterResponse{
		Status:         true,
		AccessToken:    state.AccessToken,
		ServerURL:      serverURL,
		SSHKey:         user.SSHPublicKey,
		GitHubUsername: user.GitHubUsername,
	}
	rest.SendJSON(http.StatusOK, w, response)
}

func CallRegisterAgent(client *rest.RESTClient, spaceId string) (*AgentRegisterResponse, int, error) {
	response := &AgentRegisterResponse{}
	statusCode, err := client.Post(fmt.Sprintf("/api/v1/agents/%s", spaceId), nil, response, http.StatusOK)
	return response, statusCode, err
}

type AgentStatusRequest struct {
	AgentVersion  string `json:"agent_version"`
	HasCodeServer bool   `json:"has_code_server"`
	SSHPort       int    `json:"ssh_port"`
	VNCHttpPort   int    `json:"vnc_http_port"`
	HasTerminal   bool   `json:"has_terminal"`
	TcpPorts      []int  `json:"tcp_ports"`
	HttpPorts     []int  `json:"http_ports"`
}

type AgentStatusResponse struct {
	Status bool `json:"status"`
}

func HandleAgentStatus(w http.ResponseWriter, r *http.Request) {
	cache := database.GetCacheInstance()
	spaceId := chi.URLParam(r, "space_id")

	request := AgentStatusRequest{}

	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Test if an agent is registered for the space, in RegisteredAgents map
	if state, err := cache.GetAgentState(spaceId); err == nil {
		state.AgentVersion = request.AgentVersion
		state.HasCodeServer = request.HasCodeServer
		state.SSHPort = request.SSHPort
		state.VNCHttpPort = request.VNCHttpPort
		state.HasTerminal = request.HasTerminal
		state.TcpPorts = request.TcpPorts
		state.HttpPorts = request.HttpPorts
		cache.SaveAgentState(state)

		response := AgentStatusResponse{Status: true}
		rest.SendJSON(http.StatusOK, w, response)
		return
	}

	log.Debug().Msgf("agent status for space %s not found", spaceId)
	rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "agent not found"})
}

func CallUpdateAgentStatus(client *rest.RESTClient, spaceId string, hasCodeServer bool, sshPort int, vncHttpPort int, hasTerminal bool, tcpPorts []int, httpPorts *[]int) (int, error) {
	request := &AgentStatusRequest{
		AgentVersion:  build.Version,
		HasCodeServer: hasCodeServer,
		SSHPort:       sshPort,
		VNCHttpPort:   vncHttpPort,
		HasTerminal:   hasTerminal,
		TcpPorts:      tcpPorts,
		HttpPorts:     *httpPorts,
	}
	response := &AgentStatusResponse{}
	statusCode, err := client.Put(fmt.Sprintf("/api/v1/agents/%s/status", spaceId), request, response, http.StatusOK)
	return statusCode, err
}
