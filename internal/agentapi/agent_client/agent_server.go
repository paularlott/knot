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
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/dns"
	"github.com/paularlott/knot/internal/sshd"
	"github.com/paularlott/knot/internal/util"

	"github.com/hashicorp/yamux"
	"github.com/paularlott/knot/internal/log"
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
				log.Error("maximum connection attempts reached for server , giving up", "server", s.address)

				// Remove the server from the list of servers
				s.agentClient.serverListMutex.Lock()
				delete(s.agentClient.serverList, s.address)

				// If there's no more servers in the list then inject the default server
				if len(s.agentClient.serverList) == 0 {
					log.Info("no more servers available, restarting with server", "server", s.agentClient.defaultServerAddress)

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
				hostIPs, err := dns.LookupSRV(s.address[4:])
				if err != nil || len(hostIPs) == 0 {
					log.WithError(err).Error("resolving SRV record:")
					time.Sleep(connectRetryDelay)
					s.connectionAttempts++
					continue
				}

				serverAddr = hostIPs[0].String()
			}
			log.Info("connecting to server:", "serverAddr", serverAddr)

			var err error

			// Open the connection
			cfg := config.GetAgentConfig()
			if cfg.TLS.UseTLS {
				dialer := &tls.Dialer{
					NetDialer: &net.Dialer{
						Timeout: 3 * time.Second,
					},
					Config: &tls.Config{
						InsecureSkipVerify: cfg.TLS.SkipVerify,
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
				log.WithError(err).Error("connecting to server:")
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
				log.WithError(err).Error("sending register message:")
				s.conn.Close()
				time.Sleep(connectRetryDelay)
				s.connectionAttempts++
				continue
			}

			// Wait for the register response
			var response msg.RegisterResponse
			err = msg.ReadMessage(s.conn, &response)
			if err != nil {
				log.WithError(err).Error("decoding register response:")
				s.conn.Close()
				time.Sleep(connectRetryDelay)
				s.connectionAttempts++
				continue
			}

			// If get a freeze then spin here as server going to reboot
			if response.Freeze {
				log.Info("server is going to reboot, waiting for it to start...")
				time.Sleep(40 * time.Second)
				continue
			}

			// If registration rejected, log and exit
			if !response.Success {
				log.Error("registration rejected")
				s.conn.Close()
				time.Sleep(connectRetryDelay)
				s.connectionAttempts++
				continue
			}

			log.Info("registered with server", "server", serverAddr, "version", response.Version)

			// If 1st registration then start the ssh server if required
			s.agentClient.firstRegistrationMutex.Lock()
			if s.agentClient.firstRegistration {
				s.agentClient.firstRegistration = false

				// Remember the feature flags
				s.agentClient.withTerminal = response.WithTerminal && !cfg.DisableTerminal
				s.agentClient.withVSCodeTunnel = response.WithVSCodeTunnel && cfg.VSCodeTunnel != ""
				s.agentClient.withCodeServer = response.WithCodeServer && cfg.Port.CodeServer > 0
				s.agentClient.withSSH = response.WithSSH && s.agentClient.sshPort > 0
				s.agentClient.withRunCommand = response.WithRunCommand && !cfg.DisableSpaceIO

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
						log.Info("using external ssh server on port", "port", s.agentClient.sshPort)
						conn.Close()
					}
				}

				// Fetch and start code server
				if s.agentClient.withCodeServer {
					go startCodeServer(cfg.Port.CodeServer)
				}

				// Fetch and start vscode tunnel
				if s.agentClient.withVSCodeTunnel {
					go startVSCodeTunnel(cfg.VSCodeTunnel)
				}
			}
			s.agentClient.firstRegistrationMutex.Unlock()

			// Save the keys and github usernames
			s.agentClient.lastPublicSSHKeys = response.SSHKeys
			s.agentClient.lastGitHubUsernames = response.GitHubUsernames

			// Update the authorized keys file & shell
			if s.agentClient.usingInternalSSH {
				if err := sshd.UpdateAuthorizedKeys(response.SSHKeys, response.GitHubUsernames); err != nil {
					log.WithError(err).Error("updating internal SSH server keys:")
				}
				sshd.SetShell(response.Shell)
			} else if cfg.UpdateAuthorizedKeys && s.agentClient.withSSH {
				if err := util.UpdateAuthorizedKeys(response.SSHKeys, response.GitHubUsernames); err != nil {
					log.WithError(err).Error("updating authorized keys:")
				}
			}

			// Open the mux session
			s.muxSession, err = yamux.Client(s.conn, &yamux.Config{
				AcceptBacklog:          256,
				EnableKeepAlive:        true,
				KeepAliveInterval:      30 * time.Second,
				ConnectionWriteTimeout: 2 * time.Second,
				MaxStreamWindowSize:    256 * 1024,
				StreamCloseTimeout:     6 * time.Minute,
				StreamOpenTimeout:      3 * time.Second,
				LogOutput:              io.Discard,
				//Logger:                 logger.NewMuxLogger(),
			})
			if err != nil {
				log.WithError(err).Error("creating mux session:")
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
					log.Info("context cancelled, shutting down connection to server:", "server", s.address)
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
						log.WithError(err).Error("accepting connection:")

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

	cfg := config.GetAgentConfig()

	// Read the command
	cmd, err := msg.ReadCommand(stream)
	if err != nil {
		log.WithError(err).Error("reading command:")
		return
	}

	switch cmd {
	case byte(msg.CmdPing):
		err := msg.WriteMessage(stream, &msg.Pong{
			Payload: "pong",
		})
		if err != nil {
			log.WithError(err).Error("sending pong:")
		}

	case byte(msg.CmdUpdateAuthorizedKeys):
		var updateAuthorizedKeys msg.UpdateAuthorizedKeys
		if err := msg.ReadMessage(stream, &updateAuthorizedKeys); err != nil {
			log.WithError(err).Error("reading update authorized keys message:")
			return
		}

		// Test if the keys have changed
		s.agentClient.keysMutex.Lock()
		if !reflect.DeepEqual(updateAuthorizedKeys.SSHKeys, s.agentClient.lastPublicSSHKeys) || !reflect.DeepEqual(updateAuthorizedKeys.GitHubUsernames, s.agentClient.lastGitHubUsernames) {
			s.agentClient.lastPublicSSHKeys = updateAuthorizedKeys.SSHKeys
			s.agentClient.lastGitHubUsernames = updateAuthorizedKeys.GitHubUsernames

			if s.agentClient.usingInternalSSH {
				if err := sshd.UpdateAuthorizedKeys(updateAuthorizedKeys.SSHKeys, updateAuthorizedKeys.GitHubUsernames); err != nil {
					log.WithError(err).Error("updating internal SSH server keys:")
				}
			} else if cfg.UpdateAuthorizedKeys && s.agentClient.withSSH {
				if err := util.UpdateAuthorizedKeys(updateAuthorizedKeys.SSHKeys, updateAuthorizedKeys.GitHubUsernames); err != nil {
					log.WithError(err).Error("updating authorized keys:")
				}
			}
		}
		s.agentClient.keysMutex.Unlock()

	case byte(msg.CmdUpdateShell):
		var updateShell msg.UpdateShell
		if err := msg.ReadMessage(stream, &updateShell); err != nil {
			log.WithError(err).Error("reading update shell message:")
			return
		}

		if s.agentClient.usingInternalSSH {
			sshd.SetShell(updateShell.Shell)
		}

	case byte(msg.CmdTerminal):
		var terminal msg.Terminal
		if err := msg.ReadMessage(stream, &terminal); err != nil {
			log.WithError(err).Error("reading terminal message:")
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
			ProxyTcp(stream, fmt.Sprintf("%d", cfg.Port.CodeServer))
		}

	case byte(msg.CmdProxyTCPPort):
		var tcpPort msg.TcpPort
		if err := msg.ReadMessage(stream, &tcpPort); err != nil {
			log.WithError(err).Error("reading tcp port message:")
			return
		}

		/* 		// Check if the port is allowed
		   		_, okTcp := tcpPortMap[fmt.Sprintf("%d", tcpPort.Port)]
		   		_, okHttp := httpPortMap[fmt.Sprintf("%d", tcpPort.Port)]
		   		if !okTcp && !okHttp {
		   			log.Error("tcp port  is not allowed", "port", tcpPort.Port)
		   			return
		   		} */

		ProxyTcp(stream, fmt.Sprintf("%d", tcpPort.Port))

	case byte(msg.CmdProxyVNC):
		if cfg.Port.VNCHttp > 0 {
			ProxyTcpTls(stream, fmt.Sprintf("%d", cfg.Port.VNCHttp), "127.0.0.1", true)
		}

	case byte(msg.CmdProxyHTTP):
		var httpPort msg.HttpPort
		if err := msg.ReadMessage(stream, &httpPort); err != nil {
			log.WithError(err).Error("reading tcp port message:")
			return
		}

		// Check if the port is allowed in the http map
		if _, ok := s.agentClient.httpPortMap[fmt.Sprintf("%d", httpPort.Port)]; ok {
			ProxyTcp(stream, fmt.Sprintf("%d", httpPort.Port))
		} else if _, ok := s.agentClient.httpsPortMap[fmt.Sprintf("%d", httpPort.Port)]; ok {
			ProxyTcpTls(stream, fmt.Sprintf("%d", httpPort.Port), httpPort.ServerName, true)
		} else {
			log.Error("http port  is not allowed", "port", httpPort.Port)
		}

	case byte(msg.CmdTunnelPort):
		var reversePort msg.TcpPort
		if err := msg.ReadMessage(stream, &reversePort); err != nil {
			log.WithError(err).Error("reading reverse port message:")
			return
		}

		s.agentPortListenAndServe(stream, reversePort.Port)

	case byte(msg.CmdRunCommand):
		var runCmd msg.RunCommandMessage
		if err := msg.ReadMessage(stream, &runCmd); err != nil {
			log.WithError(err).Error("reading run command message:")
			return
		}

		if s.agentClient.withRunCommand {
			handleRunCommandExecution(stream, runCmd)
		}

	case byte(msg.CmdCopyFile):
		var copyCmd msg.CopyFileMessage
		if err := msg.ReadMessage(stream, &copyCmd); err != nil {
			log.WithError(err).Error("reading copy file message:")
			return
		}

		if s.agentClient.withRunCommand {
			handleCopyFileExecution(stream, copyCmd)
		}

	case byte(msg.CmdPortForward):
		var portCmd msg.PortForwardRequest
		if err := msg.ReadMessage(stream, &portCmd); err != nil {
			log.WithError(err).Error("reading port forward message:")
			return
		}

		if s.agentClient.withRunCommand {
			handlePortForwardExecution(stream, portCmd, s.agentClient)
		}

	case byte(msg.CmdPortList):
		if s.agentClient.withRunCommand {
			handlePortListExecution(stream, s.agentClient)
		}

	case byte(msg.CmdPortStop):
		var portCmd msg.PortStopRequest
		if err := msg.ReadMessage(stream, &portCmd); err != nil {
			log.WithError(err).Error("reading port stop message:")
			return
		}

		if s.agentClient.withRunCommand {
			handlePortStopExecution(stream, portCmd, s.agentClient)
		}

	case byte(msg.CmdExecuteScript):
		var execMsg msg.ExecuteScriptMessage
		if err := msg.ReadMessage(stream, &execMsg); err != nil {
			log.WithError(err).Error("reading execute script message:")
			return
		}

		handleExecuteScript(stream, execMsg)

	default:
		log.Error("unknown command:", "cmd", cmd)
	}
}
