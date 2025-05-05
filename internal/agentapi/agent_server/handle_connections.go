package agent_server

import (
	"net"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/internal/agentapi/logger"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/cluster"

	"github.com/hashicorp/yamux"
	"github.com/rs/zerolog/log"
)

const (
	AGENT_SESSION_LOG_HISTORY = 1000 // Number of lines of log history to keep
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

		case byte(msg.CmdUpdateSpaceDescription):
			var spaceDesc msg.SpaceDescription
			if err := msg.ReadMessage(stream, &spaceDesc); err != nil {
				log.Error().Msgf("agent: reading space description message: %v", err)
				return
			}

			// Load the space from the database
			db := database.GetInstance()
			space, err := db.GetSpace(session.Id)
			if err != nil {
				log.Error().Msgf("agent: unknown space: %s", session.Id)
				return
			}

			// Update description and save it
			space.Description = spaceDesc.Description
			space.UpdatedAt = time.Now().UTC()
			if err := db.SaveSpace(space, []string{"Description", "UpdatedAt"}); err != nil {
				log.Error().Msgf("agent: updating space description: %v", err)
				return
			}

			cluster.GetInstance().GossipSpace(space)

			// Single shot command so done
			return

		default:
			log.Error().Msgf("agent: unknown command from agent: %d", cmd)
			return
		}
	}
}
