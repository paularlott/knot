package agentv1

import (
	"net/http"

	"github.com/paularlott/knot/util/rest"
	"github.com/rs/zerolog/log"
)

type AgentPingResponse struct {
  Status bool `json:"status"`
}

func HandleAgentPing(w http.ResponseWriter, r *http.Request) {

  log.Debug().Msg("Agent ping")

  rest.SendJSON(http.StatusOK, w, AgentPingResponse{
    Status: true,
  })
}

func CallAgentPing(client *rest.RESTClient) bool {
  ping := AgentPingResponse{}
  statusCode, err := client.Get("/ping", &ping)
  return statusCode == http.StatusOK && err == nil && ping.Status
}
