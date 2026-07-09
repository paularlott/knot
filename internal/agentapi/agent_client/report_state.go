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

	"github.com/paularlott/knot/internal/log"
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
		log.Fatal("failed to get user home directory", "error", err)
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
		cpuPercent, memoryUsedBytes, memoryLimitBytes, diskUsedBytes, diskLimitBytes := c.collectResourceUsage()
		activityWriteCount, activityCreateCount, activityDeleteCount, activityRenameCount, activityDistinctPaths, lastActivityAtUnix := c.snapshotActivityState()
		activityBucketStartUnix := time.Now().UTC().Truncate(time.Minute).Unix()
		activityBucketFinalized := false

		// If sshPort > 0 then check the health of sshd (until confirmed live, then assume it stays live)
		if c.withSSH && c.sshPort > 0 && sshAlivePort == 0 {
			if c.sshConfirmedLive {
				// Already confirmed live, just return the cached port
				sshAlivePort = c.sshPort
			} else {
				// Not yet confirmed, check health of sshd
				address := fmt.Sprintf("127.0.0.1:%d", c.sshPort)
				connSSH, err := net.DialTimeout("tcp", address, time.Second)
				if err == nil {
					connSSH.Close()
					sshAlivePort = c.sshPort
					c.sshConfirmedLive = true
				}
			}
		}

		// If codeServerPort > 0 then check the health of code-server, http://127.0.0.1/healthz
		if c.withCodeServer && codeServerPort > 0 {
			// Check health of code-server
			address := fmt.Sprintf("http://127.0.0.1:%d", codeServerPort)
			client, err := rest.NewClient(address, "", cfg.TLS.SkipVerify)
			if err != nil {
				log.WithError(err).Error("failed to create rest client for code-server")
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
			output, err := runCmdWithTimeout("screen", "-ls")
			if err != nil {
				log.WithError(err).Error("failed to list screen sessions")
			} else if strings.Contains(string(output), vscodeTunnelScreen) {
				hasVSCodeTunnel = true

				// Call code tunnel status to get the JSON response
				output, err := runCmdWithTimeout(codeBin, "tunnel", "status")
				if err != nil {
					log.WithError(err).Error("failed to get vscode tunnel status")
				} else {
					// Unmarshal the JSON response
					var tunnelStatus map[string]interface{}
					err := json.Unmarshal(output, &tunnelStatus)
					if err != nil {
						log.WithError(err).Error("failed to unmarshal vscode tunnel status")
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

		log.Trace("state to server",
			"SSH Port", sshAlivePort,
			"Code Server Port", codeServerAlive,
			"VNC Http Port", vncAliveHttpPort,
			"Has Terminal", c.withTerminal,
			"Has VSCode Tunnel", hasVSCodeTunnel,
			"VSCode Tunnel Name", vscodeTunnelName,
			"CPU Percent", cpuPercent,
			"Memory Used", memoryUsedBytes,
			"Memory Limit", memoryLimitBytes,
			"Disk Used", diskUsedBytes,
			"Disk Limit", diskLimitBytes,
		)

		var reportedEndpoints []string

		// Report the state to all servers
		c.serverListMutex.RLock()
		for _, server := range c.serverList {
			if server.muxSession != nil && !server.muxSession.IsClosed() {
				if server.reportingConn == nil {
					log.Debug("opening reporting connection to", "agent", server.address)

					server.reportingConn, err = server.muxSession.Open()
					if err != nil {
						log.Error("failed to open mux session for server", "server", server.address)
						continue
					}
				}

				c.healthMu.RLock()
				healthy := c.healthy
				c.healthMu.RUnlock()

				reply, err := msg.SendState(server.reportingConn, codeServerAlive, sshAlivePort, vncAliveHttpPort, c.withTerminal, &c.tcpPortMap, &webPorts, hasVSCodeTunnel, vscodeTunnelName, healthy, cpuPercent, memoryUsedBytes, memoryLimitBytes, diskUsedBytes, diskLimitBytes, activityWriteCount, activityCreateCount, activityDeleteCount, activityRenameCount, activityDistinctPaths, activityBucketStartUnix, activityBucketFinalized, lastActivityAtUnix, c.methodCallsTotal.Load(), c.httpRequestsTotal.Load(), c.tcpConnectionsTotal.Load())
				if err != nil {
					log.Error("failed to send state to server", "server", server.address)
					server.reportingConn.Close()
					server.reportingConn = nil
				} else {
					reportedEndpoints = append(reportedEndpoints, reply.Endpoints...)
				}
			}
		}
		// Filter advertised endpoints to those we should dial (not known, not in
		// the post-give-up cooldown).
		newServers := c.discoverNewServersLocked(reportedEndpoints)
		c.serverListMutex.RUnlock()

		// If we have new servers, update the server list
		if len(newServers) > 0 {
			log.Info("discovered new servers:", "newServers", newServers)
			c.serverListMutex.Lock()
			for _, newServer := range newServers {
				if c.knownServerAddresses[newServer] {
					continue
				}
				// Re-check under the write lock in case the address entered the
				// cooldown between releasing the read lock and acquiring this one.
				if c.inRediscoverCooldownLocked(newServer) {
					continue
				}
				c.clearRediscoverCooldownLocked(newServer)
				connection := NewAgentServer(newServer, c.spaceId, c)
				c.serverList[connection.address] = connection
				c.knownServerAddresses[connection.address] = true
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

func (c *AgentClient) ReportEvent(event *msg.Event) error {
	c.serverListMutex.RLock()
	defer c.serverListMutex.RUnlock()

	var delivered int
	for _, server := range c.serverList {
		if server.muxSession != nil && !server.muxSession.IsClosed() {
			conn, err := server.muxSession.Open()
			if err != nil {
				log.Error("failed to open stream for event", "server", server.address)
				continue
			}

			if err := msg.SendEvent(conn, event); err != nil {
				log.Error("failed to send event to server", "server", server.address)
				conn.Close()
				continue
			}

			conn.Close()
			delivered++
		}
	}

	if delivered == 0 {
		return fmt.Errorf("no servers accepted the event")
	}
	return nil
}

// vscodeProbeTimeout bounds the vscode-tunnel probe commands below. `screen -ls`
// and `code tunnel status` have no inherent timeout and can hang (e.g. on a
// stuck binary or network); running them without a deadline would stall
// reportState's ticker and freeze telemetry for the space.
const vscodeProbeTimeout = 3 * time.Second

// runCmdWithTimeout runs a command with a bounded timeout.
func runCmdWithTimeout(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), vscodeProbeTimeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}
