package agent_client

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/util/rest"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func ReportState(spaceId string, codeServerPort int, sshPort int, vncHttpPort int, hasTerminal bool) {
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

		// Combine http and https ports
		webPorts := make(map[string]string, len(httpPortMap)+len(httpsPortMap))
		for k, v := range httpPortMap {
			webPorts[k] = v
		}
		for k, v := range httpsPortMap {
			webPorts[k] = v
		}

		log.Debug().Msgf("agent: state to server: SSH %d, Code Server %d, VNC Http %d, Code Server Alive %t", sshAlivePort, codeServerPort, vncAliveHttpPort, codeServerAlive)

		if muxSession != nil {
			// Open a connections over the mux session and write command
			conn, err := muxSession.Open()
			if err != nil {
				log.Error().Err(err).Msg("agent: failed to open mux session")
				time.Sleep(AGENT_STATE_PING_INTERVAL)
				continue
			}

			err = msg.SendState(conn, spaceId, codeServerAlive, sshAlivePort, vncAliveHttpPort, hasTerminal, &tcpPortMap, &webPorts)
			if err != nil {
				log.Error().Err(err).Msg("agent: failed to send state to server")
			}

			conn.Close()
		}

		time.Sleep(AGENT_STATE_PING_INTERVAL)
	}
}
