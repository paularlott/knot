package agent

import (
	"time"

	"github.com/paularlott/knot/api/apiv1"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/rest"

	"github.com/rs/zerolog/log"
)

func ReportState(serverAddr string, nameserver string, spaceId string) {
  for {

    // TODO Check health of code-server and sshd, update the supported status before sending to server

    log.Debug().Msgf("Updating agent status for space %s", spaceId)

    client := rest.NewClient(util.ResolveSRVHttp(serverAddr, nameserver), middleware.AgentSpaceKey)
    statusCode, err := apiv1.CallUpdateAgentStatus(client, spaceId)
    if err != nil {
      log.Info().Msgf("failed to ping server: %s", err.Error())
      log.Info().Msgf("failed to ping server: %d", statusCode)

      // TODO Attempt registration with server
      log.Debug().Msgf("Attempting to register agent with server")
      Register(serverAddr, nameserver, spaceId)
    }

    time.Sleep(2 * time.Second) // TODO make this configurable or at least set a sane amount of time
  }
}
