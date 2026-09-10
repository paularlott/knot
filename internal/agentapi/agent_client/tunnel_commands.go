package agent_client

import (
	"net"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/agenttunnel"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
)

// handleTunnelStartExecution handles the tunnel start command from the server.
// It runs in the same process as agentlink, so it directly uses the shared
// agenttunnel registry — the same one the in-space CLI path populates. A
// request carrying its own server/token targets that knot server instead of
// the one that owns the space.
func handleTunnelStartExecution(stream net.Conn, tunnelCmd msg.TunnelStartRequest, agentClient *AgentClient) {
	server, token, skipVerify, err := agenttunnel.Credentials(tunnelCmd.Server, tunnelCmd.Token, tunnelCmd.ServerTlsSkipVerify, agentClient.GetServerURL(), agentClient.GetAgentToken(), config.GetAgentConfig().TLS.SkipVerify)
	if err != nil {
		log.Error("Failed to resolve tunnel server", "error", err)
		msg.WriteMessage(stream, &msg.TunnelStartResponse{Success: false, Error: err.Error()})
		return
	}

	url, err := agenttunnel.CreateWebTunnel(tunnelCmd.Name, tunnelCmd.Protocol, tunnelCmd.Port, tunnelCmd.TlsName, tunnelCmd.TlsSkipVerify, server, token, skipVerify)
	if err != nil {
		log.WithError(err).Error("Failed to create tunnel")
		msg.WriteMessage(stream, &msg.TunnelStartResponse{Success: false, Error: err.Error()})
		return
	}

	msg.WriteMessage(stream, &msg.TunnelStartResponse{Success: true, URL: url})
}

func handleTunnelStopExecution(stream net.Conn, tunnelCmd msg.TunnelStopRequest) {
	if _, exists := agenttunnel.Get(tunnelCmd.Name); !exists {
		msg.WriteMessage(stream, &msg.TunnelStopResponse{Success: false, Error: "tunnel not found"})
		return
	}

	agenttunnel.Stop(tunnelCmd.Name)
	msg.WriteMessage(stream, &msg.TunnelStopResponse{Success: true})
}

func handleTunnelListExecution(stream net.Conn) {
	entries := agenttunnel.List()

	response := msg.TunnelListResponse{
		Tunnels: make([]msg.TunnelInfo, 0, len(entries)),
	}
	for _, e := range entries {
		response.Tunnels = append(response.Tunnels, msg.TunnelInfo{
			Port:     e.Port,
			Protocol: e.Protocol,
			Name:     e.Name,
			URL:      e.URL,
		})
	}

	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("tunnel list: failed to write response")
	}
}
