package agentlink

import (
	"net"

	"github.com/paularlott/knot/internal/agenttunnel"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
)

func handleStartTunnel(conn net.Conn, msg *CommandMsg) {
	var request StartTunnelRequest
	if err := msg.Unmarshal(&request); err != nil {
		log.WithError(err).Error("Failed to unmarshal start tunnel request")
		sendMsg(conn, CommandNil, StartTunnelResponse{Success: false, Error: err.Error()})
		return
	}

	server := agentClient.GetServerURL()
	token := agentClient.GetAgentToken()
	if server == "" || token == "" {
		log.Error("Failed to get connection info from agent")
		sendMsg(conn, CommandNil, StartTunnelResponse{Success: false, Error: "failed to get connection info"})
		return
	}

	cfg := config.GetAgentConfig()

	url, err := agenttunnel.CreateWebTunnel(request.Name, request.Protocol, request.Port, request.TlsName, request.TlsSkipVerify, server, token, cfg.TLS.SkipVerify)
	if err != nil {
		log.WithError(err).Error("Failed to create tunnel")
		sendMsg(conn, CommandNil, StartTunnelResponse{Success: false, Error: err.Error()})
		return
	}

	sendMsg(conn, CommandNil, StartTunnelResponse{Success: true, URL: url})
}
