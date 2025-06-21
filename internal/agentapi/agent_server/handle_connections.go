package agent_server

import (
	"net"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/agentapi/logger"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"

	"github.com/hashicorp/yamux"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	AGENT_SESSION_LOG_HISTORY = 1000 // Number of lines of log history to keep
	AGENT_TOKEN_DESCRIPTION   = "agent token"
)

func handleAgentConnection(conn net.Conn) {
	defer conn.Close()

	// New connection therefore we just wait for the registration message
	var registerMsg msg.Register
	if err := msg.ReadMessage(conn, &registerMsg); err != nil {
		log.Error().Msgf("Error reading register message: %v", err)
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
	}

	// Check if the agent is already registered
	session := GetSession(registerMsg.SpaceId)
	if session != nil {

		// Ping the old agent, if it fails then delete and allow the new agent to register
		if session.Ping() {
			log.Error().Msgf("Agent already registered: %s", registerMsg.SpaceId)
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
		log.Error().Msgf("agent: unknown space: %s", registerMsg.SpaceId)
		msg.WriteMessage(conn, &response)
		return
	}

	// Load the template from the database
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		log.Error().Msgf("agent: unknown template: %s", space.TemplateId)
		msg.WriteMessage(conn, &response)
		return
	}

	// Load the user that owns the space
	user, err := db.GetUser(space.UserId)
	if err != nil {
		log.Error().Msgf("agent: unknown user: %s", space.UserId)
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
		log.Error().Msgf("Error writing register response: %v", err)
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
		Logger:                 logger.NewMuxLogger(),
	})
	if err != nil {
		log.Error().Msgf("agent: creating mux session: %v", err)
		return
	}

	// If manual template then record spaces start time
	if template.IsManual() {
		now := time.Now().UTC()
		space.UpdatedAt = now
		space.StartedAt = now
		if err := db.SaveSpace(space, []string{"UpdatedAt", "StartedAt"}); err != nil {
			log.Error().Msgf("agent: updating space start time: %v", err)
			return
		}
	}

	log.Debug().Str("space_name", space.Name).Msg("agent: session created...")

	// Loop forever waiting for connections on the mux session
	for {
		// Accept a new connection
		stream, err := session.MuxSession.Accept()
		if err != nil {

			// If error is session shutdown
			if err == yamux.ErrSessionShutdown {
				log.Info().Msgf("agent: session shutdown: %s", session.Id)
				return
			}

			log.Error().Msgf("agent: accepting connection: %v", err)
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
			log.Error().Msgf("agent: session reading command: %v", err)
			return
		}

		switch cmd {
		case byte(msg.CmdUpdateState):

			// Read the state message
			var state msg.AgentState
			if err := msg.ReadMessage(stream, &state); err != nil {
				log.Error().Msgf("agent: reading state message: %v", err)
				return
			}

			// Get the session and update the state
			if session != nil {
				session.HasCodeServer = state.HasCodeServer
				session.SSHPort = state.SSHPort
				session.VNCHttpPort = state.VNCHttpPort
				session.HasTerminal = state.HasTerminal
				session.TcpPorts = state.TcpPorts
				session.HttpPorts = state.HttpPorts
				session.HasVSCodeTunnel = state.HasVSCodeTunnel
				session.VSCodeTunnelName = state.VSCodeTunnelName
				session.AgentIp = state.AgentIp
			}

			// Return the list of agent server endpoints
			reply := msg.AgentStateReply{
				Endpoints: service.GetTransport().GetAgentEndpoints(),
			}
			if err := msg.WriteMessage(stream, &reply); err != nil {
				log.Error().Msgf("agent: writing agent state reply: %v", err)
				return
			}

		case byte(msg.CmdLogMessage):
			var logMsg msg.LogMessage
			if err := msg.ReadMessage(stream, &logMsg); err != nil {
				log.Error().Msgf("agent: reading log message: %v", err)
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
				log.Error().Msgf("agent: reading space note message: %v", err)
				return
			}

			// Load the space from the database
			db := database.GetInstance()
			space, err := db.GetSpace(session.Id)
			if err != nil {
				log.Error().Msgf("agent: unknown space: %s", session.Id)
				return
			}

			// Update note and save it
			space.Note = spaceNote.Note
			space.UpdatedAt = time.Now().UTC()
			if err := db.SaveSpace(space, []string{"Note", "UpdatedAt"}); err != nil {
				log.Error().Msgf("agent: updating space note: %v", err)
				return
			}

			service.GetTransport().GossipSpace(space)

			// Single shot command so done
			return

		case byte(msg.CmdCreateToken):
			handleCreateToken(stream, session)
			return // Single shot command so done

		default:
			log.Error().Msgf("agent: unknown command from agent: %d", cmd)
			return
		}
	}
}

func handleCreateToken(stream net.Conn, session *Session) {
	db := database.GetInstance()

	// Load the space from the database so we can get the user id
	space, err := db.GetSpace(session.Id)
	if err != nil {
		log.Error().Msgf("agent: unknown space: %s", session.Id)
		return
	}

	createTokenMutex.Lock()
	defer createTokenMutex.Unlock()

	// Get the users tokens
	tokens, err := db.GetTokensForUser(space.UserId)
	if err != nil {
		log.Error().Msgf("agent: getting tokens for user: %s", err)
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
			log.Error().Msgf("agent: saving token: %v", err)
			return
		}
		service.GetTransport().GossipToken(token)
	}

	response := msg.CreateTokenResponse{
		Server: viper.GetString("server.url"),
		Token:  token.Id,
	}
	if err := msg.WriteMessage(stream, &response); err != nil {
		log.Error().Msgf("agent: writing create token response: %v", err)
		return
	}
}
