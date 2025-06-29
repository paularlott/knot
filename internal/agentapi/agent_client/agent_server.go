package agent_client

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"reflect"
	"strings"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/sshd"
	"github.com/paularlott/knot/internal/util"

	"github.com/hashicorp/yamux"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	connectRetryDelay = 1 * time.Second // Delay before retrying connection
)

type agentServer struct {
	agentClient        *AgentClient
	connectionAttempts int // Number of connection attempts
	spaceId            string
	address            string
	conn               net.Conn
	muxSession         *yamux.Session
	ctx                context.Context
	cancel             context.CancelFunc
	reportingConn      net.Conn // Connection for reporting state
	logConn            net.Conn // Connection for logging messages
}

func NewAgentServer(address, spaceId string, agentClient *AgentClient) *agentServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &agentServer{
		agentClient:        agentClient,
		connectionAttempts: 0,
		spaceId:            spaceId,
		address:            address,
		ctx:                ctx,
		cancel:             cancel,
		muxSession:         nil,
	}
}

func (s *agentServer) ConnectAndServe() {
	go func() {
		for {

		StartConnectionLoop:

			// Check if the max connection attempts have been reached
			if s.connectionAttempts >= maxConnectionAttempts {
				log.Error().Msgf("agent: maximum connection attempts reached for server %s, giving up", s.address)

				// Remove the server from the list of servers
				s.agentClient.serverListMutex.Lock()
				delete(s.agentClient.serverList, s.address)

				// If there's no more servers in the list then inject the default server
				if len(s.agentClient.serverList) == 0 {
					log.Info().Msgf("agent: no more servers available, restarting with server %s", s.agentClient.defaultServerAddress)

					connection := NewAgentServer(s.agentClient.defaultServerAddress, s.spaceId, s.agentClient)
					s.agentClient.serverList[s.agentClient.defaultServerAddress] = connection
					connection.ConnectAndServe()
				}
				s.agentClient.serverListMutex.Unlock()

				return
			}

			// If the server address starts srv+ then resolve the SRV record
			serverAddr := s.address
			if strings.HasPrefix(s.address, "srv+") {
				hostIPs, err := util.LookupSRV(s.address[4:])
				if err != nil || len(hostIPs) == 0 {
					log.Error().Msgf("agent: resolving SRV record: %v", err)
					time.Sleep(connectRetryDelay)
					s.connectionAttempts++
					continue
				}

				serverAddr = hostIPs[0].Host + ":" + hostIPs[0].Port
			}
			log.Info().Msgf("agent: connecting to server: %s", serverAddr)

			var err error

			// Open the connection
			if viper.GetBool("agent.tls.use_tls") {
				dialer := &tls.Dialer{
					NetDialer: &net.Dialer{
						Timeout: 3 * time.Second,
					},
					Config: &tls.Config{
						InsecureSkipVerify: viper.GetBool("tls_skip_verify"),
					},
				}
				s.conn, err = dialer.Dial("tcp", serverAddr)
			} else {
				dialer := &net.Dialer{
					Timeout: 3 * time.Second,
				}
				s.conn, err = dialer.Dial("tcp", serverAddr)
			}
			if err != nil {
				log.Error().Msgf("agent: connecting to server: %v", err)
				time.Sleep(connectRetryDelay)
				s.connectionAttempts++
				continue
			}

			// Create and send the register message
			err = msg.WriteMessage(s.conn, &msg.Register{
				SpaceId: s.spaceId,
				Version: build.Version,
			})
			if err != nil {
				log.Error().Msgf("agent: sending register message: %v", err)
				s.conn.Close()
				time.Sleep(connectRetryDelay)
				s.connectionAttempts++
				continue
			}

			// Wait for the register response
			var response msg.RegisterResponse
			err = msg.ReadMessage(s.conn, &response)
			if err != nil {
				log.Error().Msgf("agent: decoding register response: %v", err)
				s.conn.Close()
				time.Sleep(connectRetryDelay)
				s.connectionAttempts++
				continue
			}

			// If registration rejected, log and exit
			if !response.Success {
				log.Error().Msgf("agent: registration rejected")
				s.conn.Close()
				time.Sleep(connectRetryDelay)
				s.connectionAttempts++
				continue
			}

			log.Info().Msgf("agent: registered with server: %s (%s)", serverAddr, response.Version)

			// If 1st registration then start the ssh server if required
			s.agentClient.firstRegistrationMutex.Lock()
			if s.agentClient.firstRegistration {
				s.agentClient.firstRegistration = false

				// Remember the feature flags
				s.agentClient.withTerminal = response.WithTerminal
				s.agentClient.withVSCodeTunnel = response.WithVSCodeTunnel && viper.GetString("agent.vscode_tunnel") != ""
				s.agentClient.withCodeServer = response.WithCodeServer && viper.GetInt("agent.port.code_server") > 0
				s.agentClient.withSSH = response.WithSSH && s.agentClient.sshPort > 0

				// If ssh port given then test if to start the ssh server
				if s.agentClient.withSSH {
					// Add the ssh port to the map
					s.agentClient.tcpPortMap[fmt.Sprintf("%d", s.agentClient.sshPort)] = "SSH"

					// Test if the ssh port is open
					conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", s.agentClient.sshPort))
					if err != nil {
						sshd.ListenAndServe(s.agentClient.sshPort, response.SSHHostSigner)
						s.agentClient.usingInternalSSH = true
					} else {
						log.Info().Msgf("agent: using external ssh server on port %d", s.agentClient.sshPort)
						conn.Close()
					}
				}

				// Fetch and start code server
				if s.agentClient.withCodeServer {
					go startCodeServer(viper.GetInt("agent.port.code_server"))
				}

				// Fetch and start vscode tunnel
				if s.agentClient.withVSCodeTunnel {
					go startVSCodeTunnel(viper.GetString("agent.vscode_tunnel"))
				}
			}
			s.agentClient.firstRegistrationMutex.Unlock()

			// Save the keys and github usernames
			s.agentClient.lastPublicSSHKeys = response.SSHKeys
			s.agentClient.lastGitHubUsernames = response.GitHubUsernames

			// Update the authorized keys file & shell
			if s.agentClient.usingInternalSSH {
				if err := sshd.UpdateAuthorizedKeys(response.SSHKeys, response.GitHubUsernames); err != nil {
					log.Error().Msgf("agent: updating internal SSH server keys: %v", err)
				}
				sshd.SetShell(response.Shell)
			} else if viper.GetBool("agent.update_authorized_keys") && s.agentClient.withSSH {
				if err := util.UpdateAuthorizedKeys(response.SSHKeys, response.GitHubUsernames); err != nil {
					log.Error().Msgf("agent: updating authorized keys: %v", err)
				}
			}

			// Open the mux session
			s.muxSession, err = yamux.Client(s.conn, &yamux.Config{
				AcceptBacklog:          256,
				EnableKeepAlive:        true,
				KeepAliveInterval:      30 * time.Second,
				ConnectionWriteTimeout: 2 * time.Second,
				MaxStreamWindowSize:    256 * 1024,
				StreamCloseTimeout:     3 * time.Minute,
				StreamOpenTimeout:      3 * time.Second,
				LogOutput:              io.Discard,
				//Logger:                 logger.NewMuxLogger(),
			})
			if err != nil {
				log.Error().Msgf("agent: creating mux session: %v", err)
				s.conn.Close()
				s.conn = nil
				time.Sleep(connectRetryDelay)
				s.connectionAttempts++
				continue
			}

			s.connectionAttempts = 0

			// Loop forever waiting for connections on the mux session
			for {
				select {
				case <-s.ctx.Done():
					log.Info().Msgf("agent: context cancelled, shutting down connection to server: %s", s.address)
					if s.reportingConn != nil {
						s.reportingConn.Close()
						s.reportingConn = nil
					}
					if s.logConn != nil {
						s.logConn.Close()
						s.logConn = nil
					}

					s.muxSession.Close()
					s.conn.Close()
					s.conn = nil
					return
				default:
					// Accept a new connection
					stream, err := s.muxSession.Accept()
					if err != nil {
						log.Error().Msgf("agent: accepting connection: %v", err)

						// In the case of errors, destroy the session and start over
						if s.reportingConn != nil {
							s.reportingConn.Close()
							s.reportingConn = nil
						}
						if s.logConn != nil {
							s.logConn.Close()
							s.logConn = nil
						}
						if s.muxSession != nil {
							s.muxSession.Close()
							s.muxSession = nil
						}
						if s.conn != nil {
							s.conn.Close()
							s.conn = nil
						}

						time.Sleep(connectRetryDelay)
						goto StartConnectionLoop
					}

					// Handle the connection
					go s.handleAgentClientStream(stream)
				}
			}
		}
	}()
}

func (s *agentServer) Shutdown() {
	s.cancel() // Cancel the context to stop the connection loop

	// Close the reporting connection if it exists
	if s.reportingConn != nil {
		s.reportingConn.Close()
		s.reportingConn = nil
	}

	// Close the log connection if it exists
	if s.logConn != nil {
		s.logConn.Close()
		s.logConn = nil
	}

	// Close the mux session if it exists
	if s.muxSession != nil {
		s.muxSession.Close()
		s.muxSession = nil
	}

	// Close the main connection if it exists
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
}

func (s *agentServer) handleAgentClientStream(stream net.Conn) {
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

		// Test if the keys have changed
		s.agentClient.keysMutex.Lock()
		if !reflect.DeepEqual(updateAuthorizedKeys.SSHKeys, s.agentClient.lastPublicSSHKeys) || !reflect.DeepEqual(updateAuthorizedKeys.GitHubUsernames, s.agentClient.lastGitHubUsernames) {
			s.agentClient.lastPublicSSHKeys = updateAuthorizedKeys.SSHKeys
			s.agentClient.lastGitHubUsernames = updateAuthorizedKeys.GitHubUsernames

			if s.agentClient.usingInternalSSH {
				if err := sshd.UpdateAuthorizedKeys(updateAuthorizedKeys.SSHKeys, updateAuthorizedKeys.GitHubUsernames); err != nil {
					log.Error().Msgf("agent: updating internal SSH server keys: %v", err)
				}
			} else if viper.GetBool("agent.update_authorized_keys") && s.agentClient.withSSH {
				if err := util.UpdateAuthorizedKeys(updateAuthorizedKeys.SSHKeys, updateAuthorizedKeys.GitHubUsernames); err != nil {
					log.Error().Msgf("agent: updating authorized keys: %v", err)
				}
			}
		}
		s.agentClient.keysMutex.Unlock()

	case byte(msg.CmdUpdateShell):
		var updateShell msg.UpdateShell
		if err := msg.ReadMessage(stream, &updateShell); err != nil {
			log.Error().Msgf("agent: reading update shell message: %v", err)
			return
		}

		if s.agentClient.usingInternalSSH {
			sshd.SetShell(updateShell.Shell)
		}

	case byte(msg.CmdTerminal):
		var terminal msg.Terminal
		if err := msg.ReadMessage(stream, &terminal); err != nil {
			log.Error().Msgf("agent: reading terminal message: %v", err)
			return
		}

		if s.agentClient.withTerminal {
			startTerminal(stream, terminal.Shell)
		}

	case byte(msg.CmdVSCodeTunnelTerminal):
		if s.agentClient.withVSCodeTunnel {
			startVSCodeTunnelTerminal(stream)
		}

	case byte(msg.CmdCodeServer):
		if s.agentClient.withCodeServer {
			ProxyTcp(stream, fmt.Sprintf("%d", viper.GetInt("agent.port.code_server")))
		}

	case byte(msg.CmdProxyTCPPort):
		var tcpPort msg.TcpPort
		if err := msg.ReadMessage(stream, &tcpPort); err != nil {
			log.Error().Msgf("agent: reading tcp port message: %v", err)
			return
		}

		/* 		// Check if the port is allowed
		   		_, okTcp := tcpPortMap[fmt.Sprintf("%d", tcpPort.Port)]
		   		_, okHttp := httpPortMap[fmt.Sprintf("%d", tcpPort.Port)]
		   		if !okTcp && !okHttp {
		   			log.Error().Msgf("agent: tcp port %d is not allowed", tcpPort.Port)
		   			return
		   		} */

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
		if _, ok := s.agentClient.httpPortMap[fmt.Sprintf("%d", httpPort.Port)]; ok {
			ProxyTcp(stream, fmt.Sprintf("%d", httpPort.Port))
		} else if _, ok := s.agentClient.httpsPortMap[fmt.Sprintf("%d", httpPort.Port)]; ok {
			ProxyTcpTls(stream, fmt.Sprintf("%d", httpPort.Port), httpPort.ServerName)
		} else {
			log.Error().Msgf("agent: http port %d is not allowed", httpPort.Port)
		}

	case byte(msg.CmdTunnelPort):
		var reversePort msg.TcpPort
		if err := msg.ReadMessage(stream, &reversePort); err != nil {
			log.Error().Msgf("agent: reading reverse port message: %v", err)
			return
		}

		s.agentPortListenAndServe(stream, reversePort.Port)

	default:
		log.Error().Msgf("agent: unknown command: %d", cmd)
	}
}
