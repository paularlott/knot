package web

import (
	"net/http"
	"time"
	_ "time/tzdata"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/startlog"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/gorilla/websocket"
	"github.com/paularlott/knot/internal/log"
)

func HandleLogsPage(w http.ResponseWriter, r *http.Request) {
	logger := log.WithGroup("agent")
	user := r.Context().Value("user").(*model.User)

	// Check if the user has permission to view logs
	if !user.HasPermission(model.PermissionUseLogs) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		showPageNotFound(w, r)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		showPageNotFound(w, r)
		return
	}

	// Check if the user has access to the space
	if space.UserId != user.Id && !space.IsSharedWith(user.Id) {
		showPageNotFound(w, r)
		return
	}

	tmpl, err := newTemplate("log.tmpl")
	if err != nil {
		logger.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var renderer string
	cfg := config.GetServerConfig()
	if cfg.TerminalWebGL {
		renderer = "webgl"
	} else {
		renderer = "canvas"
	}

	data := map[string]interface{}{
		"shell":        "",
		"renderer":     renderer,
		"spaceId":      spaceId,
		"spaceName":    space.Name,
		"assetVersion": assetVersionKey(),
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		logger.Error(err.Error())
	}
}

func HandleLogsStream(w http.ResponseWriter, r *http.Request) {
	logger := log.WithGroup("agent")
	user := r.Context().Value("user").(*model.User)

	// Check if the user has permission to view logs
	if !user.HasPermission(model.PermissionUseLogs) {
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
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the users timezone
	location, err := time.LoadLocation(user.Timezone)
	if err != nil {
		log.WithError(err).Error("Error loading location:")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ws := util.UpgradeToWS(w, r)
	if ws == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer ws.Close()

	// Monitor for the websocket closing. Only this goroutine reads; only the
	// handler writes.
	done := make(chan struct{})
	go func() {
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				logger.WithError(err).Debug("websocket closed")
				close(done)
				return
			}
		}
	}()

	// Before the space's agent connects (the space is starting, or not
	// running yet) show the server's start-up log and wait for the agent.
	agentSession := agent_server.GetSession(spaceId)
	if agentSession == nil {
		agentSession = waitForAgentSession(ws, done, space.Id, space.Name, location)
		if agentSession == nil {
			return
		}
	}

	// Register a notification channel with the session
	listenerId, channel := agentSession.RegisterLogListener()
	if channel == nil {
		return
	}
	defer agentSession.UnregisterLogListener(listenerId)

	// Write the log history to the websocket
	agentSession.LogHistoryMutex.RLock()
	for _, logMessage := range agentSession.LogHistory {
		if err := writeLogMessage(ws, logMessage, location); err != nil {
			logger.WithError(err).Error("error writing message")
			agentSession.LogHistoryMutex.RUnlock()
			return
		}
	}
	agentSession.LogHistoryMutex.RUnlock()

	// Send a marker to indicate the end of the history
	ws.WriteMessage(websocket.TextMessage, []byte{0})

	for {
		select {
		case <-done:
			return
		case logMessage, ok := <-channel:
			if !ok {
				// The agent's session ended (the space stopped).
				return
			}
			if err := writeLogMessage(ws, logMessage, location); err != nil {
				logger.WithError(err).Error("error writing message")
				return
			}
		}
	}
}

// waitForAgentSession streams the space's start-up log (internal/startlog)
// and a line for each change of state until the space's agent connects, then
// returns its session. It returns nil when the websocket closes or the space
// is deleted. The connection stays open while the space is stopped, so a log
// window opened early (or left open after a failed start) follows the next
// start too.
func waitForAgentSession(ws *websocket.Conn, done <-chan struct{}, spaceId, spaceName string, location *time.Location) *agent_server.Session {
	history, lines, cancel := startlog.Subscribe(spaceId)
	defer cancel()

	for _, logMessage := range history {
		if err := writeLogMessage(ws, logMessage, location); err != nil {
			return nil
		}
	}
	// End of history: the window now knows the stream is live.
	ws.WriteMessage(websocket.TextMessage, []byte{0})

	status := func(text string) bool {
		return writeLogMessage(ws, &msg.LogMessage{Level: msg.LogLevelInfo, Service: startlog.Service, Message: "\033[90m" + text + "\033[0m", Date: time.Now()}, location) == nil
	}

	db := database.GetInstance()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastState := ""
	for {
		if session := agent_server.GetSession(spaceId); session != nil {
			if lastState != "" && !status("Agent connected") {
				return nil
			}
			return session
		}

		space, err := db.GetSpace(spaceId)
		if err != nil || space == nil || space.IsDeleted {
			status(spaceName + " no longer exists.")
			return nil
		}

		var state, text string
		switch {
		case space.IsDeleting:
			state, text = "deleting", spaceName+" is being deleted."
		case space.IsPending && !space.IsDeployed:
			state, text = "starting", "Waiting for "+spaceName+" to start…"
		case space.IsPending:
			state, text = "stopping", spaceName+" is stopping."
		case space.IsDeployed:
			state, text = "connecting", "Waiting for "+spaceName+"'s agent to connect…"
		default:
			state = "stopped"
			if lastState == "starting" || lastState == "connecting" {
				text = spaceName + " stopped before its agent connected. Its log will continue here if it starts again."
			} else {
				text = spaceName + " isn't running. Its log will appear here when it starts."
			}
		}
		if state != lastState {
			if !status(text) {
				return nil
			}
			lastState = state
		}

		select {
		case <-done:
			return nil
		case logMessage, ok := <-lines:
			if ok {
				if err := writeLogMessage(ws, logMessage, location); err != nil {
					return nil
				}
			}
		case <-ticker.C:
		}
	}
}

func writeLogMessage(ws *websocket.Conn, logMessage *msg.LogMessage, location *time.Location) error {
	// Add the date and time in the users timezone
	prefix := "\033[90m" + logMessage.Date.In(location).Format("02 Jan 06 15:04:05 MST") + "\033[0m "

	// Style the service
	if logMessage.Service != "" {
		prefix = prefix + "\033[93m" + logMessage.Service + "\033[0m "
	}

	switch logMessage.Level {
	case msg.LogLevelDebug:
		prefix = prefix + "\033[94mDBG\033[0m "
	case msg.LogLevelInfo:
		prefix = prefix + "\033[92mINF\033[0m "
	case msg.LogLevelError:
		prefix = prefix + "\033[91mERR\033[0m "
	}

	return ws.WriteMessage(websocket.TextMessage, []byte(prefix+logMessage.Message+"\r\n"))
}
