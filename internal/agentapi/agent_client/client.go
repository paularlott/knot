package agent_client

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/sshd"
	"github.com/paularlott/knot/util"

	"github.com/hashicorp/yamux"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	AGENT_STATE_PING_INTERVAL = 2 * time.Second
)

var (
	muxSession         *yamux.Session = nil
	lastPublicSSHKey   string         = ""
	lastGitHubUsername string         = ""

	sshPort          int
	usingInteralSSH  bool = false
	withTerminal     bool = false
	withVSCodeTunnel bool = false
	withCodeServer   bool = false
	withSSH          bool = false
	httpPortMap      map[string]string
	httpsPortMap     map[string]string
	tcpPortMap       map[string]string
)

func ConnectAndServe(server string, spaceId string) {
	var firstRegistration = true

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

	// Init log message transport
	initLogMessages()

	go func() {
		for {

			// If the server address starts srv+ then resolve the SRV record
			serverAddr := server
			if strings.HasPrefix(server, "srv+") {
				hostIPs, err := util.LookupSRV(server[4:])
				if err != nil || len(*hostIPs) == 0 {
					log.Error().Msgf("agent: resolving SRV record: %v", err)
					time.Sleep(3 * time.Second)
					continue
				}

				serverAddr = (*hostIPs)[0].Host + ":" + (*hostIPs)[0].Port
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

			// Check the versions, the major and minor versions must match
			originVersionParts := strings.Split(response.Version, ".")
			agentVersionParts := strings.Split(build.Version, ".")
			if len(originVersionParts) < 2 || len(agentVersionParts) < 2 || originVersionParts[0] != agentVersionParts[0] || originVersionParts[1] != agentVersionParts[1] {
				log.Fatal().Str("origin version", response.Version).Str("leaf version", build.Version).Msg("agent: server and agent must run the same major and minor versions.")
			}

			log.Info().Msgf("agent: registered with server: %s (%s)", serverAddr, response.Version)

			// If 1st registration then start the ssh server if required
			if firstRegistration {
				firstRegistration = false

				// Remember the feature flags
				withTerminal = response.WithTerminal
				withVSCodeTunnel = response.WithVSCodeTunnel && viper.GetString("agent.vscode_tunnel") != ""
				withCodeServer = response.WithCodeServer && viper.GetInt("agent.port.code_server") > 0
				withSSH = response.WithSSH && sshPort > 0

				// If ssh port given then test if to start the ssh server
				if withSSH {
					// Add the ssh port to the map
					tcpPortMap[fmt.Sprintf("%d", sshPort)] = "SSH"

					// Test if the ssh port is open
					conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", sshPort))
					if err != nil {
						sshd.ListenAndServe(sshPort, response.SSHHostSigner)
						usingInteralSSH = true
					} else {
						log.Info().Msgf("agent: using external ssh server on port %d", sshPort)
						conn.Close()
					}
				}

				// Fetch and start code server
				if withCodeServer {
					go startCodeServer(viper.GetInt("agent.port.code_server"))
				}

				// Fetch and start vscode tunnel
				if withVSCodeTunnel {
					go startVSCodeTunnel(viper.GetString("agent.vscode_tunnel"))
				}
			}

			// Update the authorized keys file & shell
			if usingInteralSSH {
				if err := sshd.UpdateAuthorizedKeys(response.SSHKey, response.GitHubUsername); err != nil {
					log.Error().Msgf("agent: updating internal SSH server keys: %v", err)
				}
				sshd.SetShell(response.Shell)
			} else if viper.GetBool("agent.update_authorized_keys") && withSSH {
				if err := util.UpdateAuthorizedKeys(response.SSHKey, response.GitHubUsername); err != nil {
					log.Error().Msgf("agent: updating authorized keys: %v", err)
				}
			}

			// Open the mux session
			muxSession, err = yamux.Client(conn, &yamux.Config{
				AcceptBacklog:          256,
				EnableKeepAlive:        true,
				KeepAliveInterval:      30 * time.Second,
				ConnectionWriteTimeout: 10 * time.Second,
				MaxStreamWindowSize:    256 * 1024,
				StreamCloseTimeout:     5 * time.Minute,
				StreamOpenTimeout:      75 * time.Second,
				LogOutput:              io.Discard,
				//Logger:                 logger.NewMuxLogger(),
			})
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
	case byte(msg.CmdPing):
		err := msg.WriteMessage(stream, &msg.Pong{
			Payload: "pong",
		})
		if err != nil {
			log.Error().Msgf("agent: sending pong: %v", err)
		}

	case byte(msg.CmdUpdateAuthorizedKeys):
		var updateAuthorizedKeys msg.UpdateAuthorizedKeys
		if err := msg.ReadMessage(stream, &updateAuthorizedKeys); err != nil {
			log.Error().Msgf("agent: reading update authorized keys message: %v", err)
			return
		}

		if usingInteralSSH {
			if updateAuthorizedKeys.SSHKey != lastPublicSSHKey || updateAuthorizedKeys.GitHubUsername != lastGitHubUsername {
				lastPublicSSHKey = updateAuthorizedKeys.SSHKey
				lastGitHubUsername = updateAuthorizedKeys.GitHubUsername

				if err := sshd.UpdateAuthorizedKeys(updateAuthorizedKeys.SSHKey, updateAuthorizedKeys.GitHubUsername); err != nil {
					log.Error().Msgf("agent: updating internal SSH server keys: %v", err)
				}
			}
		} else if viper.GetBool("agent.update_authorized_keys") && withSSH {
			if updateAuthorizedKeys.SSHKey != lastPublicSSHKey || updateAuthorizedKeys.GitHubUsername != lastGitHubUsername {
				lastPublicSSHKey = updateAuthorizedKeys.SSHKey
				lastGitHubUsername = updateAuthorizedKeys.GitHubUsername

				if err := util.UpdateAuthorizedKeys(updateAuthorizedKeys.SSHKey, updateAuthorizedKeys.GitHubUsername); err != nil {
					log.Error().Msgf("agent: updating authorized keys: %v", err)
				}
			}
		}

	case byte(msg.CmdUpdateShell):
		var updateShell msg.UpdateShell
		if err := msg.ReadMessage(stream, &updateShell); err != nil {
			log.Error().Msgf("agent: reading update shell message: %v", err)
			return
		}

		if usingInteralSSH {
			sshd.SetShell(updateShell.Shell)
		}

	case byte(msg.CmdTerminal):
		var terminal msg.Terminal
		if err := msg.ReadMessage(stream, &terminal); err != nil {
			log.Error().Msgf("agent: reading terminal message: %v", err)
			return
		}

		if withTerminal {
			startTerminal(stream, terminal.Shell)
		}

	case byte(msg.CmdVSCodeTunnelTerminal):
		if withVSCodeTunnel {
			startVSCodeTunnelTerminal(stream)
		}

	case byte(msg.CmdCodeServer):
		if withCodeServer {
			ProxyTcp(stream, fmt.Sprintf("%d", viper.GetInt("agent.port.code_server")))
		}

	case byte(msg.CmdProxyTCPPort):
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

		ProxyTcp(stream, fmt.Sprintf("%d", tcpPort.Port))

	case byte(msg.CmdProxyVNC):
		if viper.GetUint16("agent.port.vnc_http") > 0 {
			ProxyTcpTls(stream, viper.GetString("agent.port.vnc_http"), "127.0.0.1")
		}

	case byte(msg.CmdProxyHTTP):
		var httpPort msg.HttpPort
		if err := msg.ReadMessage(stream, &httpPort); err != nil {
			log.Error().Msgf("agent: reading tcp port message: %v", err)
			return
		}

		// Check if the port is allowed in the http map
		if _, ok := httpPortMap[fmt.Sprintf("%d", httpPort.Port)]; ok {
			ProxyTcp(stream, fmt.Sprintf("%d", httpPort.Port))
		} else if _, ok := httpsPortMap[fmt.Sprintf("%d", httpPort.Port)]; ok {
			ProxyTcpTls(stream, fmt.Sprintf("%d", httpPort.Port), httpPort.ServerName)
		} else {
			log.Error().Msgf("agent: http port %d is not allowed", httpPort.Port)
		}

	default:
		log.Error().Msgf("agent: unknown command: %d", cmd)
	}
}
