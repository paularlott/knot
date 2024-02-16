package agent

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/paularlott/knot/api/apiv1"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/rest"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func ReportState(serverAddr string, nameserver string, spaceId string, codeServerPort int, sshPort int, vncHttpPort int, tcpPorts []int, httpPorts []int) {
  var failCount = 0

  // Register the agent with the server
  Register(serverAddr, nameserver, spaceId)

  for {
    var sshAlivePort = 0
    var vncAliveHttpPort = 0
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
      client := rest.NewClient(address, "", viper.GetBool("tls_skip_verify"))
      statusCode, _ := client.Get("/healthz", nil)
      if statusCode == http.StatusOK {
        codeServerAlive = true
      }
    }

    // If vncHttpPort > 0 then check the health of VNC
    if vncHttpPort > 0 {
      // Check health of sshd
      address := fmt.Sprintf("127.0.0.1:%d", vncHttpPort)
      conn, err := net.DialTimeout("tcp", address, time.Second)
      if err == nil {
        conn.Close()
        vncAliveHttpPort = vncHttpPort
      }
    }

    log.Debug().Msgf("Report agent state to server: SSH %d, Code Server %d, VNC Http %d, Code Server Alive %t", sshAlivePort, codeServerPort, vncAliveHttpPort, codeServerAlive)

    client := rest.NewClient(util.ResolveSRVHttp(middleware.ServerURL, nameserver), middleware.AgentSpaceKey, viper.GetBool("tls_skip_verify"))
    statusCode, err := apiv1.CallUpdateAgentStatus(client, spaceId, codeServerAlive, sshAlivePort, vncAliveHttpPort, viper.GetBool("agent.enable_terminal"), tcpPorts, httpPorts)
    if err != nil {
      log.Info().Msgf("failed to ping server: %d, %s", statusCode, err.Error())
      failCount++

      if failCount >= 3 {
        // Attempt reregistration with server
        log.Debug().Msgf("Attempting to register agent with server")
        Register(serverAddr, nameserver, spaceId)
        failCount = 0
      }
    } else {
      failCount = 0
    }

    time.Sleep(model.AGENT_STATE_PING_INTERVAL)
  }
}
