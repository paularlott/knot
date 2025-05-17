package agent_client

import (
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
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func ReportState() {
	var codeServerPort int = viper.GetInt("agent.port.code_server")
	var vncHttpPort int = viper.GetInt("agent.port.vnc_http")
	var vscodeTunnelScreen string = viper.GetString("agent.vscode_tunnel")
	var conn net.Conn
	var err error

	// Path to vscode binary
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal().Err(err).Msg("agent: failed to get user home directory")
	}

	codeBin := filepath.Join(homeDir, ".local", "bin", "code")

	// Find our IP address
	agentIp := viper.GetString("agent.advertise_addr")
	if agentIp == "" {
		var err error
		agentIp, err = util.GetLocalIP()
		if err != nil {
			log.Fatal().Err(err).Msg("agent: failed to get local IP address")
			return
		}
	}

	log.Info().Msgf("agent: advertising IP address %s", agentIp)

	for {
		if muxSession == nil {
			time.Sleep(2 * time.Second)
			continue
		}

		// Open a connections over the mux session and write command
		conn, err = muxSession.Open()
		if err != nil {
			log.Error().Err(err).Msg("agent: failed to open mux session")
			time.Sleep(AGENT_STATE_PING_INTERVAL)
			continue
		}

		var sshAlivePort = 0

		for {
			var vncAliveHttpPort = 0
			var codeServerAlive bool = false
			var hasVSCodeTunnel bool = false
			var vscodeTunnelName string = ""

			// If sshPort > 0 then check the health of sshd, waits for SSHD to be up
			if withSSH && sshPort > 0 && sshAlivePort == 0 {
				// Check health of sshd
				address := fmt.Sprintf("127.0.0.1:%d", sshPort)
				connSSH, err := net.DialTimeout("tcp", address, time.Second)
				if err == nil {
					connSSH.Close()
					sshAlivePort = sshPort
				}
			}

			// If codeServerPort > 0 then check the health of code-server, http://127.0.0.1/healthz
			if withCodeServer && codeServerPort > 0 {
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
				connVNC, err := net.DialTimeout("tcp", address, time.Second)
				if err == nil {
					connVNC.Close()
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

			// If using vscode tunnels
			if withVSCodeTunnel && vscodeTunnelScreen != "" {
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

			log.Debug().
				Int("SSH Port", sshAlivePort).
				Bool("Code Server Port", codeServerAlive).
				Int("VNC Http Port", vncAliveHttpPort).
				Bool("Has Terminal", withTerminal).
				Bool("Has VSCode Tunnel", hasVSCodeTunnel).
				Str("VSCode Tunnel Name", vscodeTunnelName).
				Str("Agent IP", agentIp).
				Msg("agent: state to server")

			err = msg.SendState(conn, codeServerAlive, sshAlivePort, vncAliveHttpPort, withTerminal, &tcpPortMap, &webPorts, hasVSCodeTunnel, vscodeTunnelName, agentIp)
			if err != nil {
				log.Error().Err(err).Msg("agent: failed to send state to server")
				conn.Close()
				break
			}

			time.Sleep(AGENT_STATE_PING_INTERVAL)
		}
	}
}
