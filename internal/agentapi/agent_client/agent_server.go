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
	"github.com/paularlott/knot/internal/agentapi/agentproxy"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/dns"
	"github.com/paularlott/knot/internal/portforward"
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
	reportingConn      net.Conn   // Connection for reporting state
	logConn            net.Conn   // Connection for logging messages
	logChannel         chan *msg.LogMessage

	// aliases is the set of addresses this server contributed to the client's
	// knownServerAddresses set: its dial address plus any canonical endpoint it
	// advertised at registration (response.AgentEndpoint). When the agent gives
	// up on this server these are cleared so the server can be rediscovered if
	// it comes back online. Guarded by agentClient.serverListMutex.
	aliases map[string]bool
}

func NewAgentServer(address, spaceId string, agentClient *AgentClient) *agentServer {
	ctx, cancel := context.WithCancel(context.Background())
	s := &agentServer{
		agentClient:        agentClient,
		connectionAttempts: 0,
		spaceId:            spaceId,
		address:            address,
		ctx:                ctx,
		cancel:             cancel,
		muxSession:         nil,
		logChannel:         make(chan *msg.LogMessage, logChannelBufferSize),
		aliases:            map[string]bool{address: true},
	}
	go s.logWorker()
	return s
}

// abandonLocked removes this server from the client's serverList and clears
// every address it contributed to knownServerAddresses. This lets the server
// be rediscovered from another peer's advertised endpoints if it comes back
// online — without it a server that's been given up on stays permanently in
// knownServerAddresses and the discovery filter in reportState() never
// re-adds it. The caller must hold agentClient.serverListMutex.
func (s *agentServer) abandonLocked() {
	delete(s.agentClient.serverList, s.address)
	aliases := make([]string, 0, len(s.aliases))
	for alias := range s.aliases {
		delete(s.agentClient.knownServerAddresses, alias)
		aliases = append(aliases, alias)
	}
	// Hold a rediscovery cooldown for every address this server answered to so
	// the discovery loop doesn't immediately re-dial a server we just gave up on.
	s.agentClient.markRediscoverCooldownLocked(aliases...)
}

// setConn publishes the freshly dialled connection under serverListMutex so
// the reader goroutines observe it consistently.
func (s *agentServer) setConn(conn net.Conn) {
	s.agentClient.serverListMutex.Lock()
	s.conn = conn
	s.agentClient.serverListMutex.Unlock()
}

// setMux publishes a newly established mux session under serverListMutex.
// Reader goroutines (reportState, the method server, space ops, ...) read
// s.muxSession while holding serverListMutex. Writing it here without taking
// the write lock is a data race: after a reconnect those readers can keep
// observing the previous, closed session and never resume sending state,
// which presents as "mux ping succeeds but agent state is stale".
func (s *agentServer) setMux(mux *yamux.Session) {
	s.agentClient.serverListMutex.Lock()
	s.muxSession = mux
	s.agentClient.serverListMutex.Unlock()
}

// teardownConnectionsLocked closes and clears all per-connection state. The
// caller must hold serverListMutex.
func (s *agentServer) teardownConnectionsLocked() {
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
}

// teardownConnections closes and clears all per-connection state under
// serverListMutex so readers don't race with the teardown and the next
// reconnect starts from a clean slate.
func (s *agentServer) teardownConnections() {
	s.agentClient.serverListMutex.Lock()
	s.teardownConnectionsLocked()
	s.agentClient.serverListMutex.Unlock()
}

func (s *agentServer) ConnectAndServe() {
	go func() {
		for {

		StartConnectionLoop:

			// Check if the max connection attempts have been reached
			if s.connectionAttempts >= maxConnectionAttempts {
				log.Error("maximum connection attempts reached for server , giving up", "server", s.address)

				// Remove the server from the list of servers and clear the addresses
				// it contributed to knownServerAddresses so it can be rediscovered if
				// it comes back online.
				s.agentClient.serverListMutex.Lock()
				s.abandonLocked()

				// If there's no more servers in the list then inject the default server
				if len(s.agentClient.serverList) == 0 {
					log.Info("no more servers available, restarting with server", "server", s.agentClient.defaultServerAddress)

					connection := NewAgentServer(s.agentClient.defaultServerAddress, s.spaceId, s.agentClient)
					s.agentClient.serverList[s.agentClient.defaultServerAddress] = connection
					s.agentClient.knownServerAddresses[s.agentClient.defaultServerAddress] = true
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
			var conn net.Conn
			if cfg.TLS.UseTLS {
				dialer := &tls.Dialer{
					NetDialer: &net.Dialer{
						Timeout: 3 * time.Second,
					},
					Config: &tls.Config{
						InsecureSkipVerify: cfg.TLS.SkipVerify,
					},
				}
				conn, err = dialer.Dial("tcp", serverAddr)
			} else {
				dialer := &net.Dialer{
					Timeout: 3 * time.Second,
				}
				conn, err = dialer.Dial("tcp", serverAddr)
			}
			if err != nil {
				log.WithError(err).Error("connecting to server:")
				time.Sleep(connectRetryDelay)
				s.connectionAttempts++
				continue
			}
			s.setConn(conn)

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
			if response.AgentEndpoint != "" {
				s.agentClient.serverListMutex.Lock()
				s.agentClient.knownServerAddresses[response.AgentEndpoint] = true
				s.aliases[response.AgentEndpoint] = true
				s.agentClient.serverListMutex.Unlock()
			}

			// Store the agent token and server URL at AgentClient level
			// Note: All servers in the zone generate identical tokens (deterministic HMAC)
			// so we only need to store once, on first successful registration
			s.agentClient.credentialsMutex.Lock()
			if s.agentClient.agentToken == "" {
				s.agentClient.agentToken = response.AgentToken
				s.agentClient.serverURL = response.ServerURL
			}
			s.agentClient.credentialsMutex.Unlock()

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

				// Restore persistent port forwards from server
				if len(response.PortForwards) > 0 && s.agentClient.withRunCommand {
					go s.restorePortForwards(response.PortForwards)
				}
			}
			s.agentClient.firstRegistrationMutex.Unlock()

			// Update health check config on every registration (template may have changed)
			s.agentClient.UpdateHealthCheckConfig(msg.HealthConfig{
				HealthCheckType:          response.HealthCheckType,
				HealthCheckConfig:        response.HealthCheckConfig,
				HealthCheckSkipSSLVerify: response.HealthCheckSkipSSLVerify,
				HealthCheckTimeout:       response.HealthCheckTimeout,
				HealthCheckInterval:      response.HealthCheckInterval,
				HealthCheckMaxFailures:   response.HealthCheckMaxFailures,
				HealthCheckAutoRestart:   response.HealthCheckAutoRestart,
			})

			// Save the keys and github usernames
			s.agentClient.lastPublicSSHKeys = response.SSHKeys
			s.agentClient.lastPrivateSSHKey = response.SSHPrivateKey
			s.agentClient.lastGitHubUsernames = response.GitHubUsernames

			// Update the authorized keys file, private key & shell
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
			if s.agentClient.withSSH {
				if err := util.UpdateSSHPrivateKey(response.SSHPrivateKey); err != nil {
					log.WithError(err).Error("updating SSH private key:")
				}
			}

			// Open the mux session
		mux, err := yamux.Client(s.conn, &yamux.Config{
			AcceptBacklog:          256,
			EnableKeepAlive:        true,
			KeepAliveInterval:      30 * time.Second,
			ConnectionWriteTimeout: 30 * time.Second,
			MaxStreamWindowSize:    4 * 1024 * 1024, // 4MB — 256KB default is too small for file transfers
			StreamCloseTimeout:     6 * time.Minute,
			StreamOpenTimeout:      3 * time.Second,
			LogOutput:              io.Discard,
			//Logger:                 logger.NewMuxLogger(),
		})
			if err != nil {
				log.WithError(err).Error("creating mux session:")
				s.teardownConnections()
				time.Sleep(connectRetryDelay)
				s.connectionAttempts++
				continue
			}
			s.setMux(mux)

			s.connectionAttempts = 0

			// Re-publish methods in case the knot server restarted and lost the
			// in-memory registry. No-op on first connect (nothing published yet)
			// and harmless to call repeatedly — the registry treats it as a
			// replace. Runs in a goroutine so a slow server doesn't block the
			// Accept loop.
			go s.agentClient.republishMethods()

			// Loop forever waiting for connections on the mux session
			for {
				select {
				case <-s.ctx.Done():
					log.Info("context cancelled, shutting down connection to server:", "server", s.address)
					s.teardownConnections()
					return
				default:
					// Accept a new connection
					stream, err := s.muxSession.Accept()
					if err != nil {
						log.WithError(err).Error("accepting connection:")

						// In the case of errors, destroy the session and start over
						s.teardownConnections()

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

	// Close and clear the connections. Shutdown is called by AgentClient.Shutdown
	// while it already holds serverListMutex, so use the locked variant.
	s.teardownConnectionsLocked()
}

func (s *agentServer) logWorker() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case logMsg := <-s.logChannel:
			if logMsg == nil {
				continue
			}

			// muxSession/logConn are mutated by the connection goroutine under
			// serverListMutex; read them under the same lock so a reconnect is
			// observed and we don't race the teardown.
			s.agentClient.serverListMutex.RLock()
			if s.muxSession != nil && !s.muxSession.IsClosed() {
				if s.logConn == nil {
					log.Debug("opening logging connection to", "agent", s.address)

					var err error
					s.logConn, err = s.muxSession.Open()
					if err != nil {
						log.Error("failed to open mux session for server", "server", s.address)
						s.agentClient.serverListMutex.RUnlock()
						continue
					}
				}

				if s.logConn != nil {
					err := msg.SendLogMessage(s.logConn, logMsg)
					if err != nil {
						log.Error("failed to send log message to server", "server", s.address)
						s.logConn.Close()
						s.logConn = nil
					}
				}
			}
			s.agentClient.serverListMutex.RUnlock()
		}
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

	case byte(msg.CmdUpdateHealthConfig):
		var healthConfig msg.HealthConfig
		if err := msg.ReadMessage(stream, &healthConfig); err != nil {
			log.WithError(err).Error("reading health config message:")
			return
		}
		s.agentClient.UpdateHealthCheckConfig(healthConfig)
		log.Info("updated health check config", "type", healthConfig.HealthCheckType)

	case byte(msg.CmdUpdateAuthorizedKeys):
		var updateAuthorizedKeys msg.UpdateAuthorizedKeys
		if err := msg.ReadMessage(stream, &updateAuthorizedKeys); err != nil {
			log.WithError(err).Error("reading update authorized keys message:")
			return
		}

		// Test if the keys have changed
		s.agentClient.keysMutex.Lock()

		authKeysChanged := !reflect.DeepEqual(updateAuthorizedKeys.SSHKeys, s.agentClient.lastPublicSSHKeys) || !reflect.DeepEqual(updateAuthorizedKeys.GitHubUsernames, s.agentClient.lastGitHubUsernames)
		privateKeyChanged := strings.TrimSpace(updateAuthorizedKeys.SSHPrivateKey) != strings.TrimSpace(s.agentClient.lastPrivateSSHKey)

		if authKeysChanged || privateKeyChanged {
			s.agentClient.lastPublicSSHKeys = updateAuthorizedKeys.SSHKeys
			s.agentClient.lastPrivateSSHKey = updateAuthorizedKeys.SSHPrivateKey
			s.agentClient.lastGitHubUsernames = updateAuthorizedKeys.GitHubUsernames

			if authKeysChanged {
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

			if privateKeyChanged && s.agentClient.withSSH {
				if err := util.UpdateSSHPrivateKey(updateAuthorizedKeys.SSHPrivateKey); err != nil {
					log.WithError(err).Error("updating SSH private key:")
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
			agentproxy.ProxyTcp(stream, fmt.Sprintf("%d", cfg.Port.CodeServer))
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

		s.agentClient.tcpConnectionsTotal.Add(1)
		agentproxy.ProxyTcp(stream, fmt.Sprintf("%d", tcpPort.Port))

	case byte(msg.CmdProxyVNC):
		if cfg.Port.VNCHttp > 0 {
			agentproxy.ProxyTcpTls(stream, fmt.Sprintf("%d", cfg.Port.VNCHttp), "127.0.0.1", true)
		}

	case byte(msg.CmdProxyHTTP):
		var httpPort msg.HttpPort
		if err := msg.ReadMessage(stream, &httpPort); err != nil {
			log.WithError(err).Error("reading tcp port message:")
			return
		}

		// Check if the port is allowed in the http map
		if _, ok := s.agentClient.httpPortMap[fmt.Sprintf("%d", httpPort.Port)]; ok {
			s.agentClient.httpRequestsTotal.Add(1)
			agentproxy.ProxyTcp(stream, fmt.Sprintf("%d", httpPort.Port))
		} else if _, ok := s.agentClient.httpsPortMap[fmt.Sprintf("%d", httpPort.Port)]; ok {
			s.agentClient.httpRequestsTotal.Add(1)
			agentproxy.ProxyTcpTls(stream, fmt.Sprintf("%d", httpPort.Port), httpPort.ServerName, true)
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

	case byte(msg.CmdGrep):
		var g msg.GrepMessage
		if err := msg.ReadMessage(stream, &g); err != nil {
			log.WithError(err).Error("reading grep message:")
			return
		}
		if s.agentClient.withRunCommand {
			handleGrepExecution(stream, g)
		}

	case byte(msg.CmdFind):
		var f msg.FindMessage
		if err := msg.ReadMessage(stream, &f); err != nil {
			log.WithError(err).Error("reading find message:")
			return
		}
		if s.agentClient.withRunCommand {
			handleFindExecution(stream, f)
		}

	case byte(msg.CmdSed):
		var sd msg.SedMessage
		if err := msg.ReadMessage(stream, &sd); err != nil {
			log.WithError(err).Error("reading sed message:")
			return
		}
		if s.agentClient.withRunCommand {
			handleSedExecution(stream, sd)
		}

	case byte(msg.CmdEditFile):
		var e msg.EditFileMessage
		if err := msg.ReadMessage(stream, &e); err != nil {
			log.WithError(err).Error("reading edit message:")
			return
		}
		if s.agentClient.withRunCommand {
			handleEditFileExecution(stream, e)
		}

	case byte(msg.CmdDeleteFile):
		var d msg.DeleteFileMessage
		if err := msg.ReadMessage(stream, &d); err != nil {
			log.WithError(err).Error("reading delete file message:")
			return
		}
		if s.agentClient.withRunCommand {
			handleDeleteFileExecution(stream, d)
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

	case byte(msg.CmdThrottlePort):
		var throttleCmd msg.ThrottlePortRequest
		if err := msg.ReadMessage(stream, &throttleCmd); err != nil {
			log.WithError(err).Error("reading throttle port message:")
			return
		}

		if s.agentClient.withRunCommand {
			handleThrottlePortExecution(stream, throttleCmd)
		}

	case byte(msg.CmdExecuteScript):
		var execMsg msg.ExecuteScriptMessage
		if err := msg.ReadMessage(stream, &execMsg); err != nil {
			log.WithError(err).Error("reading execute script message:")
			return
		}

		handleExecuteScript(stream, execMsg)

	case byte(msg.CmdExecuteScriptStream):
		var execMsg msg.ExecuteScriptStreamMessage
		if err := msg.ReadMessage(stream, &execMsg); err != nil {
			log.WithError(err).Error("reading execute script stream message:")
			return
		}

		handleExecuteScriptStream(stream, execMsg)

	case byte(msg.CmdCallMethod):
		var callMsg msg.CallMethodRequest
		if err := msg.ReadMessage(stream, &callMsg); err != nil {
			log.WithError(err).Error("reading call method message:")
			return
		}

		handleCallMethodExecution(stream, s.agentClient, callMsg)

	case byte(msg.CmdCallMethodBatch):
		var batchMsg msg.CallMethodBatchRequest
		if err := msg.ReadMessage(stream, &batchMsg); err != nil {
			log.WithError(err).Error("reading call method batch message:")
			return
		}

		handleCallMethodBatchExecution(stream, s.agentClient, batchMsg)

	case byte(msg.CmdTunnelStart):
		var tunnelCmd msg.TunnelStartRequest
		if err := msg.ReadMessage(stream, &tunnelCmd); err != nil {
			log.WithError(err).Error("reading tunnel start message:")
			return
		}

		if s.agentClient.withRunCommand {
			handleTunnelStartExecution(stream, tunnelCmd, s.agentClient)
		}

	case byte(msg.CmdTunnelStop):
		var tunnelCmd msg.TunnelStopRequest
		if err := msg.ReadMessage(stream, &tunnelCmd); err != nil {
			log.WithError(err).Error("reading tunnel stop message:")
			return
		}

		if s.agentClient.withRunCommand {
			handleTunnelStopExecution(stream, tunnelCmd)
		}

	case byte(msg.CmdTunnelList):
		if s.agentClient.withRunCommand {
			handleTunnelListExecution(stream)
		}

	default:
		log.Error("unknown command:", "cmd", cmd)
	}
}

func (s *agentServer) restorePortForwards(forwards []model.PortForwardEntry) {
	for _, entry := range forwards {
		server := s.agentClient.GetServerURL()
		token := s.agentClient.GetAgentToken()
		if server == "" || token == "" {
			log.Error("cannot restore port forwards: missing credentials")
			return
		}

		cfg := config.GetAgentConfig()

		if portforward.IsPortForwarded(entry.LocalPort) {
			log.Warn("skipping port forward restore, port already forwarded", "local_port", entry.LocalPort)
			continue
		}

		forwardCtx, cancel := context.WithCancel(context.Background())
		info := portforward.StartForward(entry.LocalPort, entry.RemotePort, entry.Space, cancel)
		portforward.MarkPersistent(entry.LocalPort)

		go func(e model.PortForwardEntry) {
			listener := portforward.RunTCPForwarderViaAgentWithContext(
				forwardCtx,
				server,
				fmt.Sprintf("127.0.0.1:%d", e.LocalPort),
				e.Space,
				int(e.RemotePort),
				token,
				cfg.TLS.SkipVerify,
			)
			if listener != nil {
				portforward.StoreListener(e.LocalPort, listener)
			}

			<-forwardCtx.Done()
			portforward.StopForwardIfMatch(e.LocalPort, info)
		}(entry)

		log.Info("restored persistent port forward", "local_port", entry.LocalPort, "space", entry.Space, "remote_port", entry.RemotePort)
	}
}
