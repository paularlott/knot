package web

import (
	"net/http"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

type RunCommandRequest struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
	Workdir string `json:"workdir,omitempty"`
}

func HandleRunCommandStream(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	// Check if the user has permission to run commands
	if !user.HasPermission(model.PermissionRunCommands) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && space.SharedWithUserId != user.Id) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Check if the template allows run commands
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil || template == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if !template.WithRunCommand {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	ws := util.UpgradeToWS(w, r)
	if ws == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer ws.Close()

	// Get the agent session
	agentSession := agent_server.GetSession(spaceId)
	if agentSession == nil {
		w.WriteHeader(http.StatusNotFound)
		ws.Close()
		return
	}

	// Read the command request from the websocket
	var request RunCommandRequest
	err = ws.ReadJSON(&request)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read command request")
		ws.WriteMessage(websocket.TextMessage, []byte("Error: Failed to read command request\r\n"))
		return
	}

	// Validate the request
	if request.Command == "" {
		ws.WriteMessage(websocket.TextMessage, []byte("Error: Command cannot be empty\r\n"))
		return
	}

	if request.Timeout <= 0 {
		request.Timeout = 30 // Default timeout
	}

	// Send the run command message to the agent
	runCommandMsg := &msg.RunCommandMessage{
		Command: request.Command,
		Timeout: request.Timeout,
		Workdir: request.Workdir,
	}

	// Send the command to the agent and get the response channel
	responseChannel, err := agentSession.SendRunCommand(runCommandMsg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send run command to agent")
		ws.WriteMessage(websocket.TextMessage, []byte("Error: Failed to send command to agent\r\n"))
		return
	}

	// Monitor for the websocket closing
	done := make(chan bool)
	go func() {
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				log.Debug().Msgf("websocket closed: %s", err)
				done <- true
				return
			}
		}
	}()

	// Wait for the command response or websocket close
	select {
	case response := <-responseChannel:
		if response.Success {
			// Send the command output
			if len(response.Output) > 0 {
				ws.WriteMessage(websocket.TextMessage, response.Output)
			}
		} else {
			// Send the error message
			errorMsg := "Error: " + response.Error + "\r\n"
			ws.WriteMessage(websocket.TextMessage, []byte(errorMsg))
		}

		// Send end marker
		ws.WriteMessage(websocket.TextMessage, []byte{0})

	case <-done:
		// Websocket was closed
		return
	}
}
