package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/validate"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func HandleExecuteScriptStream(w http.ResponseWriter, r *http.Request) {
	spaceId := r.PathValue("space_id")
	scriptName := r.URL.Query().Get("script")
	isContent := r.URL.Query().Get("content") == "true"
	
	if !validate.UUID(spaceId) {
		http.Error(w, "Invalid space ID", http.StatusBadRequest)
		return
	}

	if !isContent && !validate.VarName(scriptName) {
		http.Error(w, "Invalid script name", http.StatusBadRequest)
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	space, err := db.GetSpace(spaceId)
	if err != nil || space.IsDeleted {
		http.Error(w, "Space not found", http.StatusNotFound)
		return
	}

	if !user.HasPermission(model.PermissionManageSpaces) && space.UserId != user.Id {
		http.Error(w, "No permission to access this space", http.StatusForbidden)
		return
	}

	var scriptContent string
	var scriptId string

	if !isContent {
		script, err := service.ResolveScriptByName(scriptName, user.Id)
		if err != nil {
			http.Error(w, "Script not found", http.StatusNotFound)
			return
		}

		if !service.CanUserExecuteScript(user, script) {
			http.Error(w, "No permission to execute this script", http.StatusForbidden)
			return
		}

		scriptContent = script.Content
		scriptId = script.Id
		scriptName = script.Name
	}

	session := agent_server.GetSession(space.Id)
	if session == nil {
		http.Error(w, "Space agent is not connected", http.StatusServiceUnavailable)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	// If content mode, read script content from first message
	if isContent {
		msgType, data, err := ws.ReadMessage()
		if err != nil || msgType != websocket.TextMessage {
			ws.WriteMessage(websocket.TextMessage, []byte("error:failed to read script content"))
			return
		}
		scriptContent = string(data)
		scriptName = "inline"
	}

	// Streaming scripts run without timeout
	// Parse arguments from query string
	args := r.URL.Query()["arg"]

	execMsg := &msg.ExecuteScriptStreamMessage{
		Content:   scriptContent,
		Arguments: args,
	}

	agentConn, err := session.SendExecuteScriptStream(execMsg)
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("error:%s", err.Error())))
		return
	}
	defer agentConn.Close()

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventScriptExecute,
		fmt.Sprintf("Executed script %s in space %s (streaming)", scriptName, space.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"script_id":       scriptId,
			"script_name":     scriptName,
			"space_id":        space.Id,
			"space_name":      space.Name,
			"streaming":       true,
			"is_content":      isContent,
		},
	)

	// Bidirectional copy
	errChan := make(chan error, 2)

	// WebSocket -> Agent (stdin)
	go func() {
		for {
			msgType, data, err := ws.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if msgType == websocket.TextMessage {
				if string(data) == "stop" {
					agentConn.Close()
					return
				}
				if string(data) == "stdin_eof" {
					// Close the write side of the agent connection to signal EOF to script
					// For most connections, closing entirely is fine since script will exit
					agentConn.Close()
					return
				}
			}
			if msgType == websocket.BinaryMessage {
				_, err = agentConn.Write(data)
				if err != nil {
					errChan <- err
					return
				}
			}
		}
	}()

	// Agent -> WebSocket (stdout)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := agentConn.Read(buf)
			if err != nil {
				if err != io.EOF {
					errChan <- err
				}
				return
			}
			data := buf[:n]
			dataStr := string(data)
			
			// Check if data contains exit message
			if idx := strings.Index(dataStr, "\nexit:"); idx >= 0 {
				// Send data before exit message
				if idx > 0 {
					ws.WriteMessage(websocket.BinaryMessage, data[:idx])
				}
				// Send exit message as text
				exitMsg := strings.TrimSpace(dataStr[idx+1:])
				ws.WriteMessage(websocket.TextMessage, []byte(exitMsg))
				return
			}
			
			err = ws.WriteMessage(websocket.BinaryMessage, data)
			if err != nil {
				errChan <- err
				return
			}
		}
	}()

	<-errChan
}
