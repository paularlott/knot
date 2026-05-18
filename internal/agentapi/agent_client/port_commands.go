package agent_client

import (
	"context"
	"fmt"
	"net"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
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

	// Get connection info from agent
	server := agentClient.GetServerURL()
	token := agentClient.GetAgentToken()
	if server == "" || token == "" {
		log.Error("Failed to get connection info from agent")
		msg.WriteMessage(stream, &msg.PortForwardResponse{
			Success: false,
			Error:   "Failed to get connection info",
		})
		return
	}

	cfg := config.GetAgentConfig()

	// When Force is not set, validate the target space
	if !portCmd.Force {
		client, err := apiclient.NewClient(server, token, cfg.TLS.SkipVerify)
		if err != nil {
			msg.WriteMessage(stream, &msg.PortForwardResponse{Success: false, Error: "failed to create API client"})
			return
		}

		ctx := context.Background()
		currentSpace, _, err := client.GetSpace(ctx, agentClient.GetSpaceId())
		if err != nil {
			msg.WriteMessage(stream, &msg.PortForwardResponse{Success: false, Error: "failed to get current space"})
			return
		}

		spaces, _, err := client.GetSpaces(ctx, currentSpace.UserId)
		if err != nil {
			msg.WriteMessage(stream, &msg.PortForwardResponse{Success: false, Error: "failed to get spaces"})
			return
		}

		var targetSpace *apiclient.SpaceInfo
		for i := range spaces.Spaces {
			if spaces.Spaces[i].Name == portCmd.Space {
				targetSpace = &spaces.Spaces[i]
				break
			}
		}

		if targetSpace == nil {
			msg.WriteMessage(stream, &msg.PortForwardResponse{Success: false, Error: "target space not found"})
			return
		}

		if !targetSpace.IsDeployed || !targetSpace.HasState {
			msg.WriteMessage(stream, &msg.PortForwardResponse{Success: false, Error: "target space is not running"})
			return
		}
	}

	// Check if port is already forwarded
	if portforward.IsPortForwarded(portCmd.LocalPort) {
		msg.WriteMessage(stream, &msg.PortForwardResponse{
			Success: false,
			Error:   "Port already forwarded",
		})
		return
	}

	// Create context for this forward
	forwardCtx, cancel := context.WithCancel(context.Background())
	portforward.StartForward(portCmd.LocalPort, portCmd.RemotePort, portCmd.Space, cancel)

	if portCmd.Persistent {
		portforward.MarkPersistent(portCmd.LocalPort)
		if err := agentClient.AddPortForward(model.PortForwardEntry{
			LocalPort:  portCmd.LocalPort,
			Space:      portCmd.Space,
			RemotePort: portCmd.RemotePort,
		}); err != nil {
			log.WithError(err).Warn("Failed to persist port forward to server")
		}
	}

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
			Persistent: portforward.IsPersistent(fwd.LocalPort),
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
	if err := agentClient.RemovePortForward(portCmd.LocalPort); err != nil {
		log.WithError(err).Error("Failed to remove persistent port forward from server")
	}
	msg.WriteMessage(stream, &msg.PortStopResponse{
		Success: true,
	})
}
