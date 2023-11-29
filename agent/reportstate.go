package agent

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/paularlott/knot/api/apiv1"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/rest"
	"github.com/spf13/viper"

	"github.com/rs/zerolog/log"
)

func ReportState(serverAddr string, nameserver string, spaceId string, codeServerPort int, sshPort int) {
  for {
    var sshAlivePort = 0
    var codeServerAlive bool

    // If sshPort > 0 then check the health of sshd
    if sshPort > 0 {
      // Check health of sshd
      address := fmt.Sprintf("127.0.0.1:%d", sshPort)
      conn, err := net.DialTimeout("tcp", address, time.Second)
      if err == nil {
        conn.Close()
        sshAlivePort = sshPort
      }
    }

    // If codeServerPort > 0 then check the health of code-server, http://127.0.0.1/healthz
    codeServerAlive = false
    if codeServerPort > 0 {
      // Check health of code-server
      address := fmt.Sprintf("http://127.0.0.1:%d", codeServerPort)
      client := rest.NewClient(address, "")
      statusCode, _ := client.Get("/healthz", nil)
      if statusCode == http.StatusOK {
        codeServerAlive = true
      }
    }

    client := rest.NewClient(util.ResolveSRVHttp(serverAddr, nameserver), middleware.AgentSpaceKey)
    statusCode, err := apiv1.CallUpdateAgentStatus(client, spaceId, codeServerAlive, sshAlivePort, viper.GetBool("agent.enable-terminal"))
    if err != nil {
      log.Info().Msgf("failed to ping server: %d, %s", statusCode, err.Error())

      // Attempt reregistration with server
      log.Debug().Msgf("Attempting to register agent with server")
      Register(serverAddr, nameserver, spaceId)
    }

    time.Sleep(2 * time.Second) // TODO make this configurable or at least set a sane amount of time
  }
}
