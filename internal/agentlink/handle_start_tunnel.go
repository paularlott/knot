package agentlink

import (
	"fmt"
	"net"
	"strings"

	"github.com/paularlott/knot/internal/agenttunnel"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/tunnel_server"
)

func handleStartTunnel(conn net.Conn, msg *CommandMsg) {
	var request StartTunnelRequest
	if err := msg.Unmarshal(&request); err != nil {
		log.WithError(err).Error("Failed to unmarshal start tunnel request")
		sendMsg(conn, CommandNil, StartTunnelResponse{Success: false, Error: err.Error()})
		return
	}

	if request.Protocol != "http" && request.Protocol != "https" {
		sendMsg(conn, CommandNil, StartTunnelResponse{Success: false, Error: "invalid protocol, must be http or https"})
		return
	}

	if request.Port < 1 {
		sendMsg(conn, CommandNil, StartTunnelResponse{Success: false, Error: "invalid port"})
		return
	}

	// Reject a duplicate tunnel on the same name.
	if agenttunnel.IsTunneled(request.Name) {
		sendMsg(conn, CommandNil, StartTunnelResponse{Success: false, Error: "a tunnel with this name already exists"})
		return
	}

	// Connection info comes from the agent's own registration.
	server := agentClient.GetServerURL()
	token := agentClient.GetAgentToken()
	if server == "" || token == "" {
		log.Error("Failed to get connection info from agent")
		sendMsg(conn, CommandNil, StartTunnelResponse{Success: false, Error: "failed to get connection info"})
		return
	}

	// Normalise the server URL to an https/http form and derive the WS URL,
	// mirroring cmdutil.GetServerAddr.
	httpServer := server
	if !strings.HasPrefix(httpServer, "http://") && !strings.HasPrefix(httpServer, "https://") {
		httpServer = "https://" + httpServer
	}
	httpServer = strings.TrimSuffix(httpServer, "/")
	wsServer := "ws" + httpServer[4:]

	cfg := config.GetAgentConfig()

	client := tunnel_server.NewTunnelClient(
		wsServer,
		httpServer,
		token,
		cfg.TLS.SkipVerify,
		&tunnel_server.TunnelOpts{
			Type:          tunnel_server.WebTunnel,
			Protocol:      request.Protocol,
			LocalPort:     request.Port,
			TunnelName:    request.Name,
			TlsName:       request.TlsName,
			TlsSkipVerify: request.TlsSkipVerify,
		},
	)
	if err := client.ConnectAndServe(); err != nil {
		log.WithError(err).Error("Failed to create tunnel")
		sendMsg(conn, CommandNil, StartTunnelResponse{Success: false, Error: fmt.Errorf("failed to create tunnel: %w", err).Error()})
		return
	}

	if _, ok := agenttunnel.Start(request.Name, request.Port, request.Protocol, client.URL(), client); !ok {
		// Lost a race for the name; tear the client down.
		client.Shutdown()
		sendMsg(conn, CommandNil, StartTunnelResponse{Success: false, Error: "a tunnel with this name already exists"})
		return
	}

	sendMsg(conn, CommandNil, StartTunnelResponse{Success: true, URL: client.URL()})
}
