package agentlink

import (
	"context"
	"fmt"
	"net"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/portforward"
)

func handleForwardPort(conn net.Conn, msg *CommandMsg) {
	var request ForwardPortRequest
	err := msg.Unmarshal(&request)
	if err != nil {
		log.WithError(err).Error("Failed to unmarshal forward port request")
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: err.Error()})
		return
	}

	// Get connection info from agent
	server := agentClient.GetServerURL()
	token := agentClient.GetAgentToken()
	if server == "" || token == "" {
		log.Error("Failed to get connection info from agent")
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "failed to get connection info"})
		return
	}

	// Create API client
	cfg := config.GetAgentConfig()
	client, err := apiclient.NewClient(server, token, cfg.TLS.SkipVerify)
	if err != nil {
		log.WithError(err).Error("Failed to create API client")
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "failed to create API client"})
		return
	}

	// Get current space info
	ctx := context.Background()
	currentSpace, _, err := client.GetSpace(ctx, agentClient.GetSpaceId())
	if err != nil {
		log.WithError(err).Error("Failed to get current space")
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "failed to get current space"})
		return
	}

	// Get target space info
	spaces, _, err := client.GetSpaces(ctx, currentSpace.UserId)
	if err != nil {
		log.WithError(err).Error("Failed to get spaces")
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "failed to get spaces"})
		return
	}

	var targetSpace *apiclient.SpaceInfo
	for _, s := range spaces.Spaces {
		if s.Name == request.Space {
			targetSpace = &s
			break
		}
	}

	if targetSpace == nil {
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "target space not found"})
		return
	}

	// Verify target space is deployed and has an active agent
	if !targetSpace.IsDeployed || !targetSpace.HasState {
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "target space is not running"})
		return
	}

	// Verify both spaces are in the same zone
	if currentSpace.Zone != targetSpace.Zone {
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "spaces must be in the same zone"})
		return
	}

	// Verify both spaces are owned by the same user
	if currentSpace.UserId != targetSpace.UserId {
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "spaces must be owned by the same user"})
		return
	}

	// Check if port is already forwarded
	if portforward.IsPortForwarded(request.LocalPort) {
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "port already forwarded"})
		return
	}

	// Create context for this forward
	forwardCtx, cancel := context.WithCancel(context.Background())
	portforward.StartForward(request.LocalPort, request.RemotePort, request.Space, cancel)

	// Send success response immediately
	sendMsg(conn, CommandNil, RunCommandResponse{Success: true})

	// Start port forwarding in background
	go func() {
		listener := portforward.RunTCPForwarderViaAgentWithContext(
			forwardCtx,
			server,
			fmt.Sprintf("127.0.0.1:%d", request.LocalPort),
			request.Space,
			int(request.RemotePort),
			token,
			cfg.TLS.SkipVerify,
		)

		if listener == nil {
			log.Error("failed to create listener for port forward", "port", request.LocalPort)
			portforward.StopForward(request.LocalPort)
			return
		}

		// Store listener
		portforward.StoreListener(request.LocalPort, listener)

		// Wait for context cancellation
		<-forwardCtx.Done()

		// Clean up when forward stops
		portforward.StopForward(request.LocalPort)
	}()
}
