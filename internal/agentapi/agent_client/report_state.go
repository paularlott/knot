package agent_client

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/rs/zerolog/log"
)

func (c *AgentClient) reportState() {
	cfg := config.GetAgentConfig()

	var codeServerPort int = cfg.Port.CodeServer
	var vncHttpPort int = cfg.Port.VNCHttp
	var vscodeTunnelScreen string = cfg.VSCodeTunnel
	var err error

	// Path to vscode binary
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal().Err(err).Msg("agent: failed to get user home directory")
	}

	codeBin := filepath.Join(homeDir, ".local", "bin", "code")

	interval := time.NewTicker(agentStatePingInterval)
	defer interval.Stop()

	for range interval.C {
		var sshAlivePort = 0

		var vncAliveHttpPort = 0
		var codeServerAlive bool = false
		var hasVSCodeTunnel bool = false
		var vscodeTunnelName string = ""

		// If sshPort > 0 then check the health of sshd, waits for SSHD to be up
		if c.withSSH && c.sshPort > 0 && sshAlivePort == 0 {
			// Check health of sshd
			address := fmt.Sprintf("127.0.0.1:%d", c.sshPort)
			connSSH, err := net.DialTimeout("tcp", address, time.Second)
			if err == nil {
				connSSH.Close()
				sshAlivePort = c.sshPort
			}
		}

		// If codeServerPort > 0 then check the health of code-server, http://127.0.0.1/healthz
		if c.withCodeServer && codeServerPort > 0 {
			// Check health of code-server
			address := fmt.Sprintf("http://127.0.0.1:%d", codeServerPort)
			client, err := rest.NewClient(address, "", cfg.TLS.SkipVerify)
			if err != nil {
				log.Error().Err(err).Msg("agent: failed to create rest client for code-server")
			} else {
				statusCode, _ := client.Get(context.Background(), "/healthz", nil)
				if statusCode == http.StatusOK {
					codeServerAlive = true
				}
			}
		}

		// If vncHttpPort > 0 then check the health of VNC
		if vncHttpPort > 0 {
			// Check health of sshd
			address := fmt.Sprintf("127.0.0.1:%d", vncHttpPort)
			connVNC, err := net.DialTimeout("tcp", address, time.Second)
			if err == nil {
				connVNC.Close()
				vncAliveHttpPort = vncHttpPort
			}
		}

		// Combine http and https ports
		webPorts := make(map[string]string, len(c.httpPortMap)+len(c.httpsPortMap))
		for k, v := range c.httpPortMap {
			webPorts[k] = v
		}
		for k, v := range c.httpsPortMap {
			webPorts[k] = v
		}

		// If using vscode tunnels
		if c.withVSCodeTunnel && vscodeTunnelScreen != "" {
			// Check if there's a screen running with the name vscodeTunnel
			screenCmd := exec.Command("screen", "-ls")
			output, err := screenCmd.Output()
			if err != nil {
				log.Error().Err(err).Msg("agent: failed to list screen sessions")
			} else if strings.Contains(string(output), vscodeTunnelScreen) {
				hasVSCodeTunnel = true

				// Call code tunnel status to get the JSON response
				tunnelCmd := exec.Command(codeBin, "tunnel", "status")
				output, err := tunnelCmd.Output()
				if err != nil {
					log.Error().Err(err).Msg("agent: failed to get vscode tunnel status")
				} else {
					// Unmarshal the JSON response
					var tunnelStatus map[string]interface{}
					err := json.Unmarshal(output, &tunnelStatus)
					if err != nil {
						log.Error().Msgf("agent: failed to unmarshal vscode tunnel status %v", err)
					} else {
						if tunnel, ok := tunnelStatus["tunnel"].(map[string]interface{}); ok {
							// If tunnel is connected then get the name
							if tunnel["tunnel"] == "Connected" {
								vscodeTunnelName = tunnel["name"].(string)
							}
						}
					}
				}
			}
		}

		log.Trace().
			Int("SSH Port", sshAlivePort).
			Bool("Code Server Port", codeServerAlive).
			Int("VNC Http Port", vncAliveHttpPort).
			Bool("Has Terminal", c.withTerminal).
			Bool("Has VSCode Tunnel", hasVSCodeTunnel).
			Str("VSCode Tunnel Name", vscodeTunnelName).
			Msg("agent: state to server")

		var newServers []string

		// Report the state to all servers
		c.serverListMutex.RLock()
		for _, server := range c.serverList {
			if server.muxSession != nil && !server.muxSession.IsClosed() {
				if server.reportingConn == nil {
					log.Debug().Msgf("agent: opening reporting connection to %s", server.address)

					server.reportingConn, err = server.muxSession.Open()
					if err != nil {
						log.Error().Err(err).Msgf("agent: failed to open mux session for server %s", server.address)
						continue
					}
				}

				reply, err := msg.SendState(server.reportingConn, codeServerAlive, sshAlivePort, vncAliveHttpPort, c.withTerminal, &c.tcpPortMap, &webPorts, hasVSCodeTunnel, vscodeTunnelName)
				if err != nil {
					log.Error().Err(err).Msgf("agent: failed to send state to server %s", server.address)
				} else {
					// Add any new servers to the new servers list
					for _, reportedServer := range reply.Endpoints {
						if _, exists := c.serverList[reportedServer]; !exists {
							if !stringInSlice(reportedServer, newServers) {
								newServers = append(newServers, reportedServer)
							}
						}
					}
				}
			}
		}
		c.serverListMutex.RUnlock()

		// If we have new servers, update the server list
		if len(newServers) > 0 {
			log.Info().Msgf("agent: discovered new servers: %v", newServers)
			c.serverListMutex.Lock()
			for _, newServer := range newServers {
				connection := NewAgentServer(newServer, c.spaceId, c)
				c.serverList[connection.address] = connection
				connection.ConnectAndServe()
			}
			c.serverListMutex.Unlock()
		}
	}
}

func stringInSlice(str string, slice []string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
