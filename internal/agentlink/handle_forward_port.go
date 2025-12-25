package agentlink

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/proxy"
)

var (
	portForwardsMux sync.RWMutex
	portForwards    = make(map[uint16]*portForwardInfo)
)

type portForwardInfo struct {
	LocalPort  uint16
	Space      string
	RemotePort uint16
	Cancel     context.CancelFunc
	Listener   net.Listener
}

func handleForwardPort(conn net.Conn, msg *CommandMsg) {
	var request ForwardPortRequest
	err := msg.Unmarshal(&request)
	if err != nil {
		log.WithError(err).Error("Failed to unmarshal forward port request")
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: err.Error()})
		return
	}

	// Get connection info from agent
	server, token, err := agentClient.SendRequestToken()
	if err != nil {
		log.WithError(err).Error("Failed to get connection info")
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
	portForwardsMux.Lock()
	if _, exists := portForwards[request.LocalPort]; exists {
		portForwardsMux.Unlock()
		sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "port already forwarded"})
		return
	}

	// Create context for this forward
	ctx, cancel := context.WithCancel(context.Background())
	info := &portForwardInfo{
		LocalPort:  request.LocalPort,
		Space:      request.Space,
		RemotePort: request.RemotePort,
		Cancel:     cancel,
	}
	portForwards[request.LocalPort] = info
	portForwardsMux.Unlock()

	// Send success response immediately
	sendMsg(conn, CommandNil, RunCommandResponse{Success: true})

	// Start port forwarding in background
	go func() {
		listener := proxy.RunTCPForwarderViaAgentWithContext(
			ctx,
			server,
			fmt.Sprintf("127.0.0.1:%d", request.LocalPort),
			request.Space,
			int(request.RemotePort),
			token,
			cfg.TLS.SkipVerify,
		)

		if listener == nil {
			log.Error("failed to create listener for port forward", "port", request.LocalPort)
			cancel()
			return
		}

		// Store listener
		portForwardsMux.Lock()
		if fwd, exists := portForwards[request.LocalPort]; exists {
			fwd.Listener = listener
		} else {
			listener.Close()
		}
		portForwardsMux.Unlock()

		// Wait for context cancellation
		<-ctx.Done()

		// Clean up when forward stops
		portForwardsMux.Lock()
		delete(portForwards, request.LocalPort)
		portForwardsMux.Unlock()
	}()
}
