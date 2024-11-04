package agent_client

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/util"

	"github.com/hashicorp/yamux"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	AGENT_STATE_PING_INTERVAL = 4 * time.Second
)

var (
	muxSession         *yamux.Session = nil
	lastPublicSSHKey   string         = ""
	lastGitHubUsername string         = ""

	sshPort      int
	httpPortMap  map[string]string
	httpsPortMap map[string]string
	tcpPortMap   map[string]string
)

func ConnectAndServe(server string, spaceId string) {

	sshPort = viper.GetInt("agent.port.ssh")

	// Build a map of available http ports
	ports := viper.GetStringSlice("agent.port.http_port")
	httpPortMap = make(map[string]string, len(ports))
	for _, port := range ports {
		var name string
		if strings.Contains(port, "=") {
			parts := strings.Split(port, "=")
			port = parts[0]
			name = parts[1]
		} else {
			name = port
		}

		httpPortMap[port] = name
	}

	// Build a map of available https ports
	ports = viper.GetStringSlice("agent.port.https_port")
	httpsPortMap = make(map[string]string, len(ports))
	for _, port := range ports {
		var name string
		if strings.Contains(port, "=") {
			parts := strings.Split(port, "=")
			port = parts[0]
			name = parts[1]
		} else {
			name = port
		}

		httpsPortMap[port] = name
	}

	// Build a map of the available tcp ports
	ports = viper.GetStringSlice("agent.port.tcp_port")
	tcpPortMap = make(map[string]string, len(ports))
	for _, port := range ports {
		var name string
		if strings.Contains(port, "=") {
			parts := strings.Split(port, "=")
			port = parts[0]
			name = parts[1]
		} else {
			name = port
		}

		tcpPortMap[port] = name
	}

	// Add the ssh port to the map
	if sshPort != 0 {
		tcpPortMap[fmt.Sprintf("%d", sshPort)] = "SSH"
	}

	go func() {
		for {

			// If the server address starts srv+ then resolve the SRV record
			var serverAddr string = server
			if serverAddr[:4] == "srv+" {
				for i := 0; i < 10; i++ {
					hostPort, err := util.LookupSRV(serverAddr[4:])
					if err != nil {
						if i == 9 {
							log.Fatal().Err(err).Msg("agent: failed to lookup SRV record for server aborting after 10 attempts")
						} else {
							log.Error().Err(err).Msg("db: failed to lookup SRV record for server")
						}
						time.Sleep(3 * time.Second)
						continue
					}

					serverAddr = (*hostPort)[0].Host + ":" + (*hostPort)[0].Port
				}
			}

			log.Info().Msgf("agent: connecting to server: %s", serverAddr)

			var conn net.Conn
			var err error

			// Open the connection
			if viper.GetBool("agent.tls.use_tls") {
				conn, err = tls.Dial("tcp", serverAddr, &tls.Config{
					InsecureSkipVerify: viper.GetBool("tls_skip_verify"),
				})
			} else {
				conn, err = tls.Dial("tcp", serverAddr, nil)
			}
			if err != nil {
				log.Error().Msgf("agent: connecting to server: %v", err)
				time.Sleep(3 * time.Second)
				continue
			}

			// Create and send the register message
			err = msg.WriteMessage(conn, &msg.Register{
				SpaceId: spaceId,
				Version: build.Version,
			})
			if err != nil {
				log.Error().Msgf("agent: sending register message: %v", err)
				conn.Close()
				time.Sleep(3 * time.Second)
				continue
			}

			// Wait for the register response
			var response msg.RegisterResponse
			err = msg.ReadMessage(conn, &response)
			if err != nil {
				log.Error().Msgf("agent: decoding register response: %v", err)
				conn.Close()
				time.Sleep(3 * time.Second)
				continue
			}

			// If registration rejected, log and exit
			if !response.Success {
				log.Error().Msgf("agent: registration rejected")
				conn.Close()
				time.Sleep(3 * time.Second)
				continue
			}

			log.Info().Msgf("agent: registered with server: %s", serverAddr)

			// Update the authorized keys file
			if viper.GetBool("agent.update_authorized_keys") && viper.GetInt("agent.port.ssh") > 0 {
				if err := util.UpdateAuthorizedKeys(response.SSHKey, response.GitHubUsername); err != nil {
					log.Error().Msgf("agent: updating authorized keys: %v", err)
				}
			}

			// Open the mux session
			muxSession, err = yamux.Client(conn, nil)
			if err != nil {
				log.Error().Msgf("agent: creating mux session: %v", err)
				conn.Close()
				time.Sleep(3 * time.Second)
				continue
			}

			// Loop forever waiting for connections on the mux session
			for {
				// Accept a new connection
				stream, err := muxSession.Accept()
				if err != nil {
					log.Error().Msgf("agent: accepting connection: %v", err)

					// In the case of errors, destroy the session and start over
					muxSession.Close()
					conn.Close()
					time.Sleep(3 * time.Second)

					break
				}

				// Handle the connection
				go handleAgentClientStream(stream)
			}
		}
	}()
}

func Shutdown() {
	if muxSession != nil {
		muxSession.Close()
	}
}

func handleAgentClientStream(stream net.Conn) {
	defer stream.Close()

	// Read the command
	cmd, err := msg.ReadCommand(stream)
	if err != nil {
		log.Error().Msgf("agent: reading command: %v", err)
		return
	}

	switch cmd {
	case msg.MSG_PING:
		err := msg.WriteMessage(stream, &msg.Pong{
			Payload: "pong",
		})
		if err != nil {
			log.Error().Msgf("agent: sending pong: %v", err)
		}

	case msg.MSG_UPDATE_AUTHORIZED_KEYS:
		var updateAuthorizedKeys msg.UpdateAuthorizedKeys
		if err := msg.ReadMessage(stream, &updateAuthorizedKeys); err != nil {
			log.Error().Msgf("agent: reading update authorized keys message: %v", err)
			return
		}

		if viper.GetBool("agent.update_authorized_keys") && viper.GetInt("agent.port.ssh") > 0 {
			if updateAuthorizedKeys.SSHKey != lastPublicSSHKey || updateAuthorizedKeys.GitHubUsername != lastGitHubUsername {
				lastPublicSSHKey = updateAuthorizedKeys.SSHKey
				lastGitHubUsername = updateAuthorizedKeys.GitHubUsername

				if err := util.UpdateAuthorizedKeys(updateAuthorizedKeys.SSHKey, updateAuthorizedKeys.GitHubUsername); err != nil {
					log.Error().Msgf("agent: updating authorized keys: %v", err)
				}
			}
		}

	case msg.MSG_TERMINAL:
		var terminal msg.Terminal
		if err := msg.ReadMessage(stream, &terminal); err != nil {
			log.Error().Msgf("agent: reading terminal message: %v", err)
			return
		}

		if viper.GetBool("agent.enable_terminal") {
			startTerminal(stream, terminal.Shell)
		}

	case msg.MSG_CODE_SERVER:
		if viper.GetInt("agent.port.code_server") > 0 {
			proxyTcp(stream, fmt.Sprintf("%d", viper.GetInt("agent.port.code_server")))
		}

	case msg.MSG_PROXY_TCP_PORT:
		var tcpPort msg.TcpPort
		if err := msg.ReadMessage(stream, &tcpPort); err != nil {
			log.Error().Msgf("agent: reading tcp port message: %v", err)
			return
		}

		// Check if the port is allowed
		if _, ok := tcpPortMap[fmt.Sprintf("%d", tcpPort.Port)]; !ok {
			log.Error().Msgf("agent: tcp port %d is not allowed", tcpPort.Port)
			return
		}

		proxyTcp(stream, fmt.Sprintf("%d", tcpPort.Port))

	case msg.MSG_PROXY_VNC:
		if viper.GetUint16("agent.port.vnc_http") > 0 {
			proxyTcpTls(stream, viper.GetString("agent.port.vnc_http"), "127.0.0.1")
		}

	case msg.MSG_PROXY_HTTP:
		var httpPort msg.HttpPort
		if err := msg.ReadMessage(stream, &httpPort); err != nil {
			log.Error().Msgf("agent: reading tcp port message: %v", err)
			return
		}

		// Check if the port is allowed in the http map
		if _, ok := httpPortMap[fmt.Sprintf("%d", httpPort.Port)]; ok {
			proxyTcp(stream, fmt.Sprintf("%d", httpPort.Port))
		} else if _, ok := httpsPortMap[fmt.Sprintf("%d", httpPort.Port)]; ok {
			proxyTcpTls(stream, fmt.Sprintf("%d", httpPort.Port), httpPort.ServerName)
		} else {
			log.Error().Msgf("agent: http port %d is not allowed", httpPort.Port)
		}

	default:
		log.Error().Msgf("agent: unknown command: %d", cmd)
	}
}
