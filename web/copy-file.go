package web

import (
	"encoding/base64"
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func HandleCopyFileStream(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	// Check if the user has permission to copy files
	if !user.HasPermission(model.PermissionCopyFiles) {
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

	// Check if the template allows run commands (reusing same permission)
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
		return
	}

	// Read the copy file request from the websocket
	var request apiclient.CopyFileRequest
	err = ws.ReadJSON(&request)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read copy file request")
		ws.WriteMessage(websocket.TextMessage, []byte("Error: Failed to read copy file request\r\n"))
		return
	}

	// Validate the request
	if request.Direction != "to_space" && request.Direction != "from_space" {
		ws.WriteMessage(websocket.TextMessage, []byte("Error: Invalid direction, must be 'to_space' or 'from_space'\r\n"))
		return
	}

	if request.Direction == "to_space" && (request.DestPath == "" || request.Content == nil) {
		ws.WriteMessage(websocket.TextMessage, []byte("Error: dest_path and content are required for to_space direction\r\n"))
		return
	}

	if request.Direction == "from_space" && request.SourcePath == "" {
		ws.WriteMessage(websocket.TextMessage, []byte("Error: source_path is required for from_space direction\r\n"))
		return
	}

	// Send the copy file message to the agent
	copyFileMsg := &msg.CopyFileMessage{
		SourcePath: request.SourcePath,
		DestPath:   request.DestPath,
		Content:    request.Content,
		Direction:  request.Direction,
		Workdir:    request.Workdir,
	}

	// Send the command to the agent and get the response channel
	responseChannel, err := agentSession.SendCopyFile(copyFileMsg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send copy file command to agent")
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
			if request.Direction == "from_space" {
				// Send the file content back as base64 encoded string
				contentBase64 := base64.StdEncoding.EncodeToString(response.Content)
				ws.WriteJSON(map[string]interface{}{
					"success": true,
					"content": contentBase64,
				})
			} else {
				// Send success message for to_space
				ws.WriteJSON(map[string]interface{}{
					"success": true,
					"message": "File copied successfully",
				})
			}
		} else {
			// Send the error message
			ws.WriteJSON(map[string]interface{}{
				"success": false,
				"error":   response.Error,
			})
		}

	case <-done:
		// Websocket was closed
		return
	}
}
