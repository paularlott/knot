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

	server, token, skipVerify, err := agenttunnel.Credentials(request.Server, request.Token, request.ServerTlsSkipVerify, agentClient.GetServerURL(), agentClient.GetAgentToken(), config.GetAgentConfig().TLS.SkipVerify)
	if err != nil {
		log.Error("Failed to resolve tunnel server", "error", err)
		sendMsg(conn, CommandNil, StartTunnelResponse{Success: false, Error: err.Error()})
		return
	}

	url, err := agenttunnel.CreateWebTunnel(request.Name, request.Protocol, request.Port, request.TlsName, request.TlsSkipVerify, server, token, skipVerify)
	if err != nil {
		log.WithError(err).Error("Failed to create tunnel")
		sendMsg(conn, CommandNil, StartTunnelResponse{Success: false, Error: err.Error()})
		return
	}

	sendMsg(conn, CommandNil, StartTunnelResponse{Success: true, URL: url})
}
