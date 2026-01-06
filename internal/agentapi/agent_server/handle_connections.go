package agent_server

import (
	"net"
	"strings"
	"time"

	"github.com/paularlott/knot/build"
	agentlogger "github.com/paularlott/knot/internal/agentapi/logger"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/tunnel_server"

	"github.com/hashicorp/yamux"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

const (
	AGENT_SESSION_LOG_HISTORY = 1000 // Number of lines of log history to keep
	AGENT_TOKEN_DESCRIPTION   = "agent token"
)

func handleAgentConnection(conn net.Conn) {
	logger := log.WithGroup("agent")
	defer conn.Close()

	// New connection therefore we just wait for the registration message
	var registerMsg msg.Register
	if err := msg.ReadMessage(conn, &registerMsg); err != nil {
		logger.WithError(err).Error("Error reading register message:")
		return
	}

	// Create response message
	response := msg.RegisterResponse{
		Version:          build.Version,
		Success:          false,
		SSHKeys:          []string{},
		GitHubUsernames:  []string{},
		Shell:            "",
		SSHHostSigner:    "",
		WithTerminal:     false,
		WithVSCodeTunnel: false,
		WithCodeServer:   false,
		WithSSH:          false,
		Freeze:           false,
	}

	// Check if the agent is already registered
	session := GetSession(registerMsg.SpaceId)
	if session != nil {

		// Ping the old agent, if it fails then delete and allow the new agent to register
		if session.Ping() {
			logger.Error("Agent already registered:", "agent", registerMsg.SpaceId)
			msg.WriteMessage(conn, &response)
			return
		}

		// Delete the old agent session
		RemoveSession(registerMsg.SpaceId)
	}

	db := database.GetInstance()

	// Load the space from the database
	space, err := db.GetSpace(registerMsg.SpaceId)
	if err != nil {
		logger.Error("unknown space:", "space", registerMsg.SpaceId)
		msg.WriteMessage(conn, &response)
		return
	}

	// Check the version of the agent
	if !compareVersionMajorMinor(registerMsg.Version, build.Version) {
		logger.Info("version mismatch, restarting space", "agent_version", registerMsg.Version, "expected_version", build.Version)

		// Ask the agent to freeze while we reboot it
		response.Freeze = true
		msg.WriteMessage(conn, &response)

		// Restart the space
		time.Sleep(2 * time.Second)
		containerService := service.GetContainerService()
		containerService.RestartSpace(space)
		return
	}

	// Load the template from the database
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		logger.Error("unknown template:", "template", space.TemplateId)
		msg.WriteMessage(conn, &response)
		return
	}

	// Load the user that owns the space
	user, err := db.GetUser(space.UserId)
	if err != nil {
		logger.Error("unknown user:", "agent", space.UserId)
		msg.WriteMessage(conn, &response)
		return
	}

	// Create a new session and start listening
	session = NewSession(registerMsg.SpaceId, registerMsg.Version)
	sessionMutex.Lock()
	sessions[registerMsg.SpaceId] = session
	sessionMutex.Unlock()
	defer RemoveSession(registerMsg.SpaceId)

	// Return the SSH key and GitHub username
	response.Success = true
	response.Shell = space.Shell
	response.SSHHostSigner = space.SSHHostSigner
	response.WithTerminal = template.WithTerminal
	response.WithVSCodeTunnel = template.WithVSCodeTunnel
	response.WithCodeServer = template.WithCodeServer
	response.WithSSH = template.WithSSH
	response.WithRunCommand = template.WithRunCommand

	if user.SSHPublicKey != "" {
		response.SSHKeys = append(response.SSHKeys, user.SSHPublicKey)
	}
	if user.GitHubUsername != "" {
		response.GitHubUsernames = append(response.GitHubUsernames, user.GitHubUsername)
	}

	// If space shared then get the keys from the shared user
	if space.SharedWithUserId != "" {
		sharedUser, err := db.GetUser(space.SharedWithUserId)
		if err == nil {
			if sharedUser.SSHPublicKey != "" {
				response.SSHKeys = append(response.SSHKeys, sharedUser.SSHPublicKey)
			}
			if sharedUser.GitHubUsername != "" {
				response.GitHubUsernames = append(response.GitHubUsernames, sharedUser.GitHubUsername)
			}
		}
	}

	// Write the response
	if err := msg.WriteMessage(conn, &response); err != nil {
		logger.WithError(err).Error("Error writing register response:")
		return
	}

	// Open the mux session
	session.MuxSession, err = yamux.Server(conn, &yamux.Config{
		AcceptBacklog:          256,
		EnableKeepAlive:        true,
		KeepAliveInterval:      30 * time.Second,
		ConnectionWriteTimeout: 2 * time.Second,
		MaxStreamWindowSize:    256 * 1024,
		StreamCloseTimeout:     3 * time.Minute,
		StreamOpenTimeout:      3 * time.Second,
		LogOutput:              nil,
		Logger:                 agentlogger.NewMuxLogger(),
	})
	if err != nil {
		logger.WithError(err).Error("creating mux session:")
		return
	}

	// If manual template then record spaces start time
	if template.IsManual() {
		space.UpdatedAt = hlc.Now()
		space.StartedAt = time.Now().UTC()
		if err := db.SaveSpace(space, []string{"UpdatedAt", "StartedAt"}); err != nil {
			logger.WithError(err).Error("updating space start time:")
			return
		}
	}

	logger.Debug("session created", "space_name", space.Name)

	// Loop forever waiting for connections on the mux session
	for {
		// Accept a new connection
		stream, err := session.MuxSession.Accept()
		if err != nil {

			// If error is session shutdown
			if err == yamux.ErrSessionShutdown {
				logger.Info("session shutdown:", "session_id", session.Id)
				return
			}

			logger.WithError(err).Error("accepting connection:")
			return
		}

		// Handle the connection
		go handleAgentSession(stream, session)
	}
}

func handleAgentSession(stream net.Conn, session *Session) {
	defer stream.Close()

	for {

		// Read the command
		cmd, err := msg.ReadCommand(stream)
		if err != nil {
			log.WithError(err).Error("session reading command:")
			return
		}

		switch cmd {
		case byte(msg.CmdUpdateState):

			// Read the state message
			var state msg.AgentState
			if err := msg.ReadMessage(stream, &state); err != nil {
				log.WithError(err).Error("reading state message:")
				return
			}

			// Get the session and update the state
			if session != nil {
				// Check if state actually changed
				stateChanged := session.HasCodeServer != state.HasCodeServer ||
					session.SSHPort != state.SSHPort ||
					session.VNCHttpPort != state.VNCHttpPort ||
					session.HasTerminal != state.HasTerminal ||
					session.HasVSCodeTunnel != state.HasVSCodeTunnel ||
					session.VSCodeTunnelName != state.VSCodeTunnelName ||
					!mapsEqual(session.TcpPorts, state.TcpPorts) ||
					!mapsEqual(session.HttpPorts, state.HttpPorts)

				session.HasCodeServer = state.HasCodeServer
				session.SSHPort = state.SSHPort
				session.VNCHttpPort = state.VNCHttpPort
				session.HasTerminal = state.HasTerminal
				session.TcpPorts = state.TcpPorts
				session.HttpPorts = state.HttpPorts
				session.HasVSCodeTunnel = state.HasVSCodeTunnel
				session.VSCodeTunnelName = state.VSCodeTunnelName

				// Only send SSE event if state actually changed
				if stateChanged {
					db := database.GetInstance()
					space, err := db.GetSpace(session.Id)
					if err == nil {
						sse.PublishSpaceChanged(space.Id, space.UserId)
					}
				}
			}

			// Return the list of agent server endpoints
			reply := msg.AgentStateReply{
				Endpoints: service.GetTransport().GetAgentEndpoints(),
			}
			if err := msg.WriteMessage(stream, &reply); err != nil {
				log.WithError(err).Error("writing agent state reply:")
				return
			}

		case byte(msg.CmdLogMessage):
			var logMsg msg.LogMessage
			if err := msg.ReadMessage(stream, &logMsg); err != nil {
				log.WithError(err).Error("reading log message:")
				return
			}

			session.LogHistoryMutex.Lock()
			session.LogHistory = append(session.LogHistory, &logMsg)

			if len(session.LogHistory) > AGENT_SESSION_LOG_HISTORY {
				session.LogHistory = session.LogHistory[1:]
			}
			session.LogHistoryMutex.Unlock()

			// Notify all log sinks
			go func() {
				session.LogListenersMutex.RLock()
				defer session.LogListenersMutex.RUnlock()
				for _, c := range session.LogListeners {
					c <- &logMsg
				}
			}()

		case byte(msg.CmdUpdateSpaceNote):
			var spaceNote msg.SpaceNote
			if err := msg.ReadMessage(stream, &spaceNote); err != nil {
				log.WithError(err).Error("reading space note message:")
				return
			}

			// Load the space from the database
			db := database.GetInstance()
			space, err := db.GetSpace(session.Id)
			if err != nil {
				log.Error("unknown space:", "agent", session.Id)
				return
			}

			// Update note and save it
			space.Note = spaceNote.Note
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"Note", "UpdatedAt"}); err != nil {
				log.WithError(err).Error("updating space note:")
				return
			}

			service.GetTransport().GossipSpace(space)

			// Single shot command so done
			return

		case byte(msg.CmdUpdateSpaceVar):
			var spaceVar msg.SpaceVar
			if err := msg.ReadMessage(stream, &spaceVar); err != nil {
				log.WithError(err).Error("reading space var message:")
				return
			}

			// Load the space from the database
			db := database.GetInstance()
			space, err := db.GetSpace(session.Id)
			if err != nil {
				log.Error("unknown space:", "agent", session.Id)
				return
			}

			// Get the template to validate field name
			template, err := db.GetTemplate(space.TemplateId)
			if err != nil {
				log.WithError(err).Error("failed to get template:")
				return
			}

			// Check if field is defined in template
			fieldDefined := false
			for _, field := range template.CustomFields {
				if field.Name == spaceVar.Name {
					fieldDefined = true
					break
				}
			}

			if !fieldDefined {
				log.Error("custom field not defined in template:", "name", spaceVar.Name)
				return
			}

			// Find and update the custom field if it exists, or add it
			found := false
			for i := range space.CustomFields {
				if space.CustomFields[i].Name == spaceVar.Name {
					space.CustomFields[i].Value = spaceVar.Value
					found = true
					break
				}
			}

			if !found {
				space.CustomFields = append(space.CustomFields, model.SpaceCustomField{
					Name:  spaceVar.Name,
					Value: spaceVar.Value,
				})
			}

			// Save the space
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"CustomFields", "UpdatedAt"}); err != nil {
				log.WithError(err).Error("updating space var:")
				return
			}

			service.GetTransport().GossipSpace(space)

			// Single shot command so done
			return

		case byte(msg.CmdGetSpaceVar):
			var spaceGetVar msg.SpaceGetVar
			if err := msg.ReadMessage(stream, &spaceGetVar); err != nil {
				log.WithError(err).Error("reading space get var message:")
				return
			}

			// Load the space from the database
			db := database.GetInstance()
			space, err := db.GetSpace(session.Id)
			if err != nil {
				log.Error("unknown space:", "agent", session.Id)
				return
			}

			// Get the template to validate field name
			template, err := db.GetTemplate(space.TemplateId)
			if err != nil {
				log.WithError(err).Error("failed to get template:")
				return
			}

			// Check if field is defined in template
			fieldDefined := false
			for _, field := range template.CustomFields {
				if field.Name == spaceGetVar.Name {
					fieldDefined = true
					break
				}
			}

			if !fieldDefined {
				log.Error("custom field not defined in template:", "name", spaceGetVar.Name)
				return
			}

			// Find the custom field value (empty string if not set)
			value := ""
			for i := range space.CustomFields {
				if space.CustomFields[i].Name == spaceGetVar.Name {
					value = space.CustomFields[i].Value
					break
				}
			}

			// Send response
			response := msg.SpaceGetVarResponse{
				Value: value,
			}
			if err := msg.WriteMessage(stream, &response); err != nil {
				log.WithError(err).Error("writing get var response:")
				return
			}

			// Single shot command so done
			return

		case byte(msg.CmdCreateToken):
			handleCreateToken(stream, session)
			return // Single shot command so done

		case byte(msg.CmdTunnelPortConnection):
			var reversePort msg.TcpPort
			if err := msg.ReadMessage(stream, &reversePort); err != nil {
				log.WithError(err).Error("reading reverse port message:")
				return
			}

			tunnel_server.TunnelAgentPort(session.Id, reversePort.Port, stream)
			return

		case byte(msg.CmdSpaceStop):
			// Load the space from the database
			db := database.GetInstance()
			space, err := db.GetSpace(session.Id)
			if err != nil {
				log.Error("unknown space:", "agent", session.Id)
				return
			}

			service.GetContainerService().StopSpace(space)

			// Single shot command so done
			return

		case byte(msg.CmdSpaceRestart):
			// Load the space from the database
			db := database.GetInstance()
			space, err := db.GetSpace(session.Id)
			if err != nil {
				log.Error("unknown space:", "agent", session.Id)
				return
			}

			service.GetContainerService().RestartSpace(space)

			// Single shot command so done
			return

		case byte(msg.CmdRunCommand):
			handleRunCommand(stream, session)
			return // Single shot command so done

		case byte(msg.CmdCopyFile):
			handleCopyFile(stream, session)
			return // Single shot command so done

		case byte(msg.CmdPortForward):
			handlePortForward(stream, session)
			return // Single shot command so done

		case byte(msg.CmdPortList):
			handlePortList(stream, session)
			return // Single shot command so done

		case byte(msg.CmdPortStop):
			handlePortStop(stream, session)
			return // Single shot command so done

		default:
			log.Error("unknown command from agent:", "cmd", cmd)
			return
		}
	}
}

func handleCreateToken(stream net.Conn, session *Session) {
	db := database.GetInstance()

	// Load the space from the database so we can get the user id
	space, err := db.GetSpace(session.Id)
	if err != nil {
		log.Error("unknown space:", "agent", session.Id)
		return
	}

	createTokenMutex.Lock()
	defer createTokenMutex.Unlock()

	// Get the users tokens
	tokens, err := db.GetTokensForUser(space.UserId)
	if err != nil {
		log.WithError(err).Error("getting tokens for user:")
		return
	}

	// Look for a token with the name AGENT_TOKEN_DESCRIPTION, if not found we create one
	var token *model.Token
	for _, t := range tokens {
		if t.Name == AGENT_TOKEN_DESCRIPTION && !t.IsDeleted {
			token = t
			break
		}
	}

	if token == nil {
		token = model.NewToken(AGENT_TOKEN_DESCRIPTION, space.UserId)
		err := db.SaveToken(token)
		if err != nil {
			log.WithError(err).Error("saving token:")
			return
		}
		service.GetTransport().GossipToken(token)
	}

	cfg := config.GetServerConfig()
	response := msg.CreateTokenResponse{
		Server: cfg.URL,
		Token:  token.Id,
	}
	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("writing create token response:")
		return
	}
}

func handleRunCommand(stream net.Conn, session *Session) {
	// Read the run command message
	var runCmd msg.RunCommandMessage
	if err := msg.ReadMessage(stream, &runCmd); err != nil {
		log.WithError(err).Error("reading run command message:")
		return
	}

	log.Info("forwarding run command to agent", "command", runCmd.Command, "space_id", session.Id)

	// Open a new connection to the agent to send the run command
	agentConn, err := session.MuxSession.Open()
	if err != nil {
		log.WithError(err).Error("opening connection to agent:")
		response := msg.RunCommandResponse{
			Success: false,
			Error:   "Failed to connect to agent",
		}
		msg.WriteMessage(stream, &response)
		return
	}
	defer agentConn.Close()

	// Send the run command to the agent
	if err := msg.WriteCommand(agentConn, msg.CmdRunCommand); err != nil {
		log.WithError(err).Error("writing run command to agent:")
		response := msg.RunCommandResponse{
			Success: false,
			Error:   "Failed to send command to agent",
		}
		msg.WriteMessage(stream, &response)
		return
	}

	if err := msg.WriteMessage(agentConn, &runCmd); err != nil {
		log.WithError(err).Error("writing run command message to agent:")
		response := msg.RunCommandResponse{
			Success: false,
			Error:   "Failed to send command message to agent",
		}
		msg.WriteMessage(stream, &response)
		return
	}

	// Read the response from the agent
	var response msg.RunCommandResponse
	if err := msg.ReadMessage(agentConn, &response); err != nil {
		log.WithError(err).Error("reading run command response from agent:")
		response = msg.RunCommandResponse{
			Success: false,
			Error:   "Failed to read response from agent",
		}
	}

	// Forward the response back to the client
	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("writing run command response to client:")
		return
	}

	log.Info("run command completed", "command", runCmd.Command, "space_id", session.Id, "success", response.Success)
}

func handleCopyFile(stream net.Conn, session *Session) {
	// Read the copy file message
	var copyCmd msg.CopyFileMessage
	if err := msg.ReadMessage(stream, &copyCmd); err != nil {
		log.WithError(err).Error("reading copy file message:")
		return
	}

	log.Info("forwarding copy file to agent", "direction", copyCmd.Direction, "source", copyCmd.SourcePath, "dest", copyCmd.DestPath, "space_id", session.Id)

	// Open a new connection to the agent to send the copy file command
	agentConn, err := session.MuxSession.Open()
	if err != nil {
		log.WithError(err).Error("opening connection to agent:")
		response := msg.CopyFileResponse{
			Success: false,
			Error:   "Failed to connect to agent",
		}
		msg.WriteMessage(stream, &response)
		return
	}
	defer agentConn.Close()

	// Send the copy file command to the agent
	if err := msg.WriteCommand(agentConn, msg.CmdCopyFile); err != nil {
		log.WithError(err).Error("writing copy file command to agent:")
		response := msg.CopyFileResponse{
			Success: false,
			Error:   "Failed to send command to agent",
		}
		msg.WriteMessage(stream, &response)
		return
	}

	if err := msg.WriteMessage(agentConn, &copyCmd); err != nil {
		log.WithError(err).Error("writing copy file message to agent:")
		response := msg.CopyFileResponse{
			Success: false,
			Error:   "Failed to send command message to agent",
		}
		msg.WriteMessage(stream, &response)
		return
	}

	// Read the response from the agent
	var response msg.CopyFileResponse
	if err := msg.ReadMessage(agentConn, &response); err != nil {
		log.WithError(err).Error("reading copy file response from agent:")
		response = msg.CopyFileResponse{
			Success: false,
			Error:   "Failed to read response from agent",
		}
	}

	// Forward the response back to the client
	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("writing copy file response to client:")
		return
	}

	log.Info("copy file completed", "direction", copyCmd.Direction, "space_id", session.Id, "success", response.Success)
}

func handlePortForward(stream net.Conn, session *Session) {
	// Read the port forward message
	var portCmd msg.PortForwardRequest
	if err := msg.ReadMessage(stream, &portCmd); err != nil {
		log.WithError(err).Error("reading port forward message:")
		return
	}

	log.Info("forwarding port forward to agent", "local_port", portCmd.LocalPort, "space", portCmd.Space, "remote_port", portCmd.RemotePort, "space_id", session.Id)

	// Open a new connection to the agent to send the port forward command
	agentConn, err := session.MuxSession.Open()
	if err != nil {
		log.WithError(err).Error("opening connection to agent:")
		response := msg.PortForwardResponse{
			Success: false,
			Error:   "Failed to connect to agent",
		}
		msg.WriteMessage(stream, &response)
		return
	}
	defer agentConn.Close()

	// Send the port forward command to the agent
	if err := msg.WriteCommand(agentConn, msg.CmdPortForward); err != nil {
		log.WithError(err).Error("writing port forward command to agent:")
		response := msg.PortForwardResponse{
			Success: false,
			Error:   "Failed to send command to agent",
		}
		msg.WriteMessage(stream, &response)
		return
	}

	if err := msg.WriteMessage(agentConn, &portCmd); err != nil {
		log.WithError(err).Error("writing port forward message to agent:")
		response := msg.PortForwardResponse{
			Success: false,
			Error:   "Failed to send command message to agent",
		}
		msg.WriteMessage(stream, &response)
		return
	}

	// Read the response from the agent
	var response msg.PortForwardResponse
	if err := msg.ReadMessage(agentConn, &response); err != nil {
		log.WithError(err).Error("reading port forward response from agent:")
		response = msg.PortForwardResponse{
			Success: false,
			Error:   "Failed to read response from agent",
		}
	}

	// Forward the response back to the client
	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("writing port forward response to client:")
		return
	}

	log.Info("port forward completed", "local_port", portCmd.LocalPort, "space", portCmd.Space, "remote_port", portCmd.RemotePort, "space_id", session.Id, "success", response.Success)
}

func handlePortList(stream net.Conn, session *Session) {
	// Open a new connection to the agent to send the port list command
	agentConn, err := session.MuxSession.Open()
	if err != nil {
		log.WithError(err).Error("opening connection to agent:")
		response := msg.PortListResponse{
			Forwards: []msg.PortForwardInfo{},
		}
		msg.WriteMessage(stream, &response)
		return
	}
	defer agentConn.Close()

	// Send the port list command to the agent
	if err := msg.WriteCommand(agentConn, msg.CmdPortList); err != nil {
		log.WithError(err).Error("writing port list command to agent:")
		response := msg.PortListResponse{
			Forwards: []msg.PortForwardInfo{},
		}
		msg.WriteMessage(stream, &response)
		return
	}

	// Read the response from the agent
	var response msg.PortListResponse
	if err := msg.ReadMessage(agentConn, &response); err != nil {
		log.WithError(err).Error("reading port list response from agent:")
		response = msg.PortListResponse{
			Forwards: []msg.PortForwardInfo{},
		}
	}

	// Forward the response back to the client
	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("writing port list response to client:")
		return
	}
}

func handlePortStop(stream net.Conn, session *Session) {
	// Read the port stop message
	var portCmd msg.PortStopRequest
	if err := msg.ReadMessage(stream, &portCmd); err != nil {
		log.WithError(err).Error("reading port stop message:")
		return
	}

	log.Info("forwarding port stop to agent", "local_port", portCmd.LocalPort, "space_id", session.Id)

	// Open a new connection to the agent to send the port stop command
	agentConn, err := session.MuxSession.Open()
	if err != nil {
		log.WithError(err).Error("opening connection to agent:")
		response := msg.PortStopResponse{
			Success: false,
			Error:   "Failed to connect to agent",
		}
		msg.WriteMessage(stream, &response)
		return
	}
	defer agentConn.Close()

	// Send the port stop command to the agent
	if err := msg.WriteCommand(agentConn, msg.CmdPortStop); err != nil {
		log.WithError(err).Error("writing port stop command to agent:")
		response := msg.PortStopResponse{
			Success: false,
			Error:   "Failed to send command to agent",
		}
		msg.WriteMessage(stream, &response)
		return
	}

	if err := msg.WriteMessage(agentConn, &portCmd); err != nil {
		log.WithError(err).Error("writing port stop message to agent:")
		response := msg.PortStopResponse{
			Success: false,
			Error:   "Failed to send command message to agent",
		}
		msg.WriteMessage(stream, &response)
		return
	}

	// Read the response from the agent
	var response msg.PortStopResponse
	if err := msg.ReadMessage(agentConn, &response); err != nil {
		log.WithError(err).Error("reading port stop response from agent:")
		response = msg.PortStopResponse{
			Success: false,
			Error:   "Failed to read response from agent",
		}
	}

	// Forward the response back to the client
	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("writing port stop response to client:")
		return
	}

	log.Info("port stop completed", "local_port", portCmd.LocalPort, "space_id", session.Id, "success", response.Success)
}

func compareVersionMajorMinor(version1, version2 string) bool {
	// Split versions by dots
	parts1 := strings.Split(version1, ".")
	parts2 := strings.Split(version2, ".")

	// Need at least 2 parts for major.minor comparison
	if len(parts1) < 2 || len(parts2) < 2 {
		return false
	}

	// Compare major version (first part)
	if parts1[0] != parts2[0] {
		return false
	}

	// Compare minor version (second part)
	if parts1[1] != parts2[1] {
		return false
	}

	return true
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
