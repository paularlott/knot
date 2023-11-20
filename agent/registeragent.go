package agent

import (
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/knot/api/apiv1"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/rest"

	"github.com/rs/zerolog/log"
)

func Register(serverAddr string, nameserver string, spaceId string) {
  middleware.AgentSpaceKey = uuid.New().String()

  // Register the agent with the server
  var response *apiv1.AgentRegisterResponse
  for {
    log.Info().Msgf("attempting registering of agent with server %s", serverAddr)

    // Call the server and get the access token to use, if the server doesn't respond sleep and try again until we get it
    client := rest.NewClient(util.ResolveSRVHttp(serverAddr, nameserver), "")

    var err error
    var statusCode int
    response, statusCode, err = apiv1.CallRegisterAgent(client, spaceId)
    if err != nil {
      log.Info().Msgf("failed to register with server server: %d", statusCode)
      time.Sleep(5 * time.Second)
      continue
    }

    middleware.AgentSpaceKey = response.AccessToken
    break
  }
}
