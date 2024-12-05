package agent_server

import (
	"net"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/internal/agentapi/logger"
	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/hashicorp/yamux"
	"github.com/rs/zerolog/log"
)

const (
	AGENT_SESSION_LOG_HISTORY = 200
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
		Version:        build.Version,
		Success:        false,
		SSHKey:         "",
		GitHubUsername: "",
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

	// Return the SSH key and GitHub username
	response.Success = true
	response.SSHKey = user.SSHPublicKey
	response.GitHubUsername = user.GitHubUsername

	// Write the response
	if err := msg.WriteMessage(conn, &response); err != nil {
		log.Error().Msgf("Error writing register response: %v", err)

		// Terminate the session
		RemoveSession(registerMsg.SpaceId)
		return
	}

	// Open the mux session
	session.MuxSession, err = yamux.Server(conn, &yamux.Config{
		AcceptBacklog:          256,
		EnableKeepAlive:        true,
		KeepAliveInterval:      30 * time.Second,
		ConnectionWriteTimeout: 10 * time.Second,
		MaxStreamWindowSize:    256 * 1024,
		StreamCloseTimeout:     5 * time.Minute,
		StreamOpenTimeout:      75 * time.Second,
		LogOutput:              nil,
		Logger:                 logger.NewMuxLogger(),
	})
	if err != nil {
		log.Error().Msgf("agent: creating mux session: %v", err)
		RemoveSession(registerMsg.SpaceId)
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

			// Destroy the session
			RemoveSession(registerMsg.SpaceId)
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
			log.Error().Msgf("agent: reading command: %v", err)
			return
		}

		switch cmd {
		case msg.MSG_UPDATE_STATE:

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
				session.ExpiresAfter = time.Now().UTC().Add(AGENT_SESSION_TIMEOUT)
			}

		case msg.MSG_LOG_MSG:
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
				session.LogNotifyMutex.RLock()
				defer session.LogNotifyMutex.RUnlock()
				for _, c := range session.LogNotify {
					c <- &logMsg
				}
			}()

		default:
			log.Error().Msgf("agent: unknown command from agent: %d", cmd)
			return
		}
	}
}
