package agentlink

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
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

	cfg := config.GetAgentConfig()

	// When Force is not set, validate the target space
	if !request.Force {
		// Create API client
		client, err := apiclient.NewClient(server, token, cfg.TLS.SkipVerify)
		if err != nil {
			log.WithError(err).Error("Failed to create API client")
			sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: "failed to create API client"})
			return
		}

		// Get current space info
		ctx := context.Background()
		currentSpace, code, err := client.GetSpace(ctx, agentClient.GetSpaceId())
		if err != nil {
			log.WithError(err).Error("Failed to get current space", "status", code)
			sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: fmt.Sprintf("failed to get current space (status %d): %v", code, err)})
			return
		}

		// Get target space info
		spaces, code, err := client.GetSpaces(ctx, currentSpace.UserId, false)
		if err != nil {
			log.WithError(err).Error("Failed to get spaces", "status", code)
			sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: fmt.Sprintf("failed to get spaces (status %d): %v", code, err)})
			return
		}

		var targetSpace *apiclient.SpaceInfo
		for _, s := range spaces.Spaces {
			if s.Name == request.Space {
				targetSpace = &s
				break
			}
		}

		if targetSpace != nil {
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
		} else {
			// Not one of this user's spaces: it may be an own pool, another
			// user's space or pool (user--space), or a stored space ID. Ask
			// the server to authorize the target now — owners may reach any
			// port, everyone else only shared ports — so a disallowed or
			// mistyped target fails here instead of on every connection.
			if message := checkForwardTarget(ctx, client, request.Space, request.RemotePort); message != "" {
				sendMsg(conn, CommandNil, RunCommandResponse{Success: false, Error: message})
				return
			}
		}
	}

	// If the port is already forwarded, tear down the existing forward so the
	// new request replaces it instead of being rejected as a conflict.
	wasPersistent := portforward.IsPersistent(request.LocalPort)
	if portforward.IsPortForwarded(request.LocalPort) {
		portforward.StopForward(request.LocalPort)
	}

	// Create context for this forward
	forwardCtx, cancel := context.WithCancel(context.Background())
	info := portforward.StartForward(request.LocalPort, request.RemotePort, request.Space, cancel)

	if request.Persistent {
		portforward.MarkPersistent(request.LocalPort)
		if err := agentClient.AddPortForward(model.PortForwardEntry{
			LocalPort:  request.LocalPort,
			Space:      request.Space,
			RemotePort: request.RemotePort,
		}); err != nil {
			log.WithError(err).Warn("Failed to persist port forward to server")
		}
	} else if wasPersistent {
		// Existing forward was persistent but the replacement isn't — remove
		// the stale DB entry so it doesn't get restored on next agent start.
		if err := agentClient.RemovePortForward(request.LocalPort); err != nil {
			log.WithError(err).Warn("Failed to remove stale persistent port forward from server")
		}
	}

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
			portforward.StopForwardIfMatch(request.LocalPort, info)
			return
		}

		// Store listener
		portforward.StoreListener(request.LocalPort, listener)

		// Wait for context cancellation
		<-forwardCtx.Done()

		// Clean up only if we still own this forward (a replacement may have
		// already taken the slot).
		portforward.StopForwardIfMatch(request.LocalPort, info)
	}()
}

// checkForwardTarget asks the server whether the requesting space's owner may
// forward to the target reference and port, returning "" when allowed or the
// server's error message. The server owns the rule (owners reach any port,
// everyone else shared ports only) and re-checks on every connection; this
// only makes bad targets fail at creation with a readable reason.
func checkForwardTarget(ctx context.Context, client *apiclient.ApiClient, ref string, port uint16) string {
	path := fmt.Sprintf("/api/forward-target?target=%s&port=%d", url.QueryEscape(ref), int(port))
	code, err := client.Do(ctx, http.MethodGet, path, nil, nil)
	if err == nil && code == http.StatusOK {
		return ""
	}

	message := err.Error()
	if prefix := "unexpected status code: "; strings.HasPrefix(message, prefix) {
		message = message[len(prefix):]
		if i := strings.Index(message, ": "); i >= 0 {
			detail := message[i+2:]
			var body struct {
				Error string `json:"error"`
			}
			if json.Unmarshal([]byte(detail), &body) == nil && body.Error != "" {
				return body.Error
			}
			if strings.TrimSpace(detail) != "" {
				// The client already unwrapped the response body.
				return detail
			}
			message = message[:i]
		}
	}
	if code == 0 {
		message = "failed to check forward target: " + message
	}
	return message
}
