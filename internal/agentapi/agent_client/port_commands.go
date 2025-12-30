package agent_client

import (
	"context"
	"fmt"
	"net"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/portforward"

	"github.com/paularlott/knot/internal/log"
)

// handlePortForwardExecution handles the port forward command from the server
// This runs in the same process as agentlink, so it can directly use the shared portforward state
func handlePortForwardExecution(stream net.Conn, portCmd msg.PortForwardRequest, agentClient *AgentClient) {
	// Validate the request
	if portCmd.LocalPort < 1 || portCmd.LocalPort > 65535 {
		msg.WriteMessage(stream, &msg.PortForwardResponse{
			Success: false,
			Error:   "Invalid local port, must be between 1 and 65535",
		})
		return
	}

	if portCmd.RemotePort < 1 || portCmd.RemotePort > 65535 {
		msg.WriteMessage(stream, &msg.PortForwardResponse{
			Success: false,
			Error:   "Invalid remote port, must be between 1 and 65535",
		})
		return
	}

	if portCmd.Space == "" {
		msg.WriteMessage(stream, &msg.PortForwardResponse{
			Success: false,
			Error:   "Target space name is required",
		})
		return
	}

	// Check if port is already forwarded
	if portforward.IsPortForwarded(portCmd.LocalPort) {
		msg.WriteMessage(stream, &msg.PortForwardResponse{
			Success: false,
			Error:   "Port already forwarded",
		})
		return
	}

	// Get connection info from agent
	server, token, err := agentClient.SendRequestToken()
	if err != nil {
		log.WithError(err).Error("Failed to get connection info")
		msg.WriteMessage(stream, &msg.PortForwardResponse{
			Success: false,
			Error:   "Failed to get connection info",
		})
		return
	}

	cfg := config.GetAgentConfig()

	// Create context for this forward
	forwardCtx, cancel := context.WithCancel(context.Background())
	portforward.StartForward(portCmd.LocalPort, portCmd.RemotePort, portCmd.Space, cancel)

	// Send success response immediately
	msg.WriteMessage(stream, &msg.PortForwardResponse{
		Success: true,
	})

	// Start port forwarding in background
	go func() {
		listener := portforward.RunTCPForwarderViaAgentWithContext(
			forwardCtx,
			server,
			fmt.Sprintf("127.0.0.1:%d", portCmd.LocalPort),
			portCmd.Space,
			int(portCmd.RemotePort),
			token,
			cfg.TLS.SkipVerify,
		)
		if listener != nil {
			portforward.StoreListener(portCmd.LocalPort, listener)
		}

		// Wait for context cancellation
		<-forwardCtx.Done()

		// Clean up when forward stops
		portforward.StopForward(portCmd.LocalPort)
	}()
}

// handlePortListExecution handles the port list command from the server
// This runs in the same process as agentlink, so it can directly use the shared portforward state
func handlePortListExecution(stream net.Conn, agentClient *AgentClient) {
	forwards := portforward.ListForwards()

	response := msg.PortListResponse{
		Forwards: make([]msg.PortForwardInfo, len(forwards)),
	}
	for i, fwd := range forwards {
		response.Forwards[i] = msg.PortForwardInfo{
			LocalPort:  fwd.LocalPort,
			Space:      fwd.Space,
			RemotePort: fwd.RemotePort,
		}
	}

	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("port list: failed to write response")
	}
}

// handlePortStopExecution handles the port stop command from the server
// This runs in the same process as agentlink, so it can directly use the shared portforward state
func handlePortStopExecution(stream net.Conn, portCmd msg.PortStopRequest, agentClient *AgentClient) {
	// Check if port forward exists
	if _, exists := portforward.GetForward(portCmd.LocalPort); !exists {
		msg.WriteMessage(stream, &msg.PortStopResponse{
			Success: false,
			Error:   "Port forward not found",
		})
		return
	}

	portforward.StopForward(portCmd.LocalPort)
	msg.WriteMessage(stream, &msg.PortStopResponse{
		Success: true,
	})
}
