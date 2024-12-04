package web

import (
	"net/http"
	"time"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/util"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func HandleLogsPage(w http.ResponseWriter, r *http.Request) {
	spaceId := chi.URLParam(r, "space_id")
	user := r.Context().Value("user").(*model.User)

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		showPageNotFound(w, r)
		return
	}

	// Check if the user has access to the space
	if space.UserId != user.Id {
		showPageNotFound(w, r)
		return
	}

	tmpl, err := newTemplate("log.tmpl")
	if err != nil {
		log.Fatal().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var renderer string
	if viper.GetBool("server.terminal.webgl") {
		renderer = "webgl"
	} else {
		renderer = "canvas"
	}

	data := map[string]interface{}{
		"shell":    "",
		"renderer": renderer,
		"spaceId":  spaceId,
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}

func HandleLogsStream(w http.ResponseWriter, r *http.Request) {
	spaceId := chi.URLParam(r, "space_id")
	user := r.Context().Value("user").(*model.User)

	// Check user has permission to view logs
	if !user.HasPermission(model.PermissionViewLogs) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || space.UserId != user.Id {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the users timezone
	location, err := time.LoadLocation(user.Timezone)
	if err != nil {
		log.Error().Msgf("Error loading location: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
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

	// Register a notification channel with the session
	messages := agentSession.RegisterLogSink(spaceId)
	if messages == nil {
		w.WriteHeader(http.StatusConflict)
		return
	}

	// Monitor for the websocket closing
	go func() {
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				log.Debug().Msgf("websocket closed: %s", err)
				agentSession.UnregisterLogSink(spaceId)
				return
			}
		}
	}()

	// Write the log history to the websocket
	agentSession.LogHistoryMutex.RLock()
	for _, logMessage := range agentSession.LogHistory {
		if err := writeLogMessage(ws, logMessage, location); err != nil {
			log.Error().Msgf("agent: error writing message: %s", err)
			return
		}
	}
	agentSession.LogHistoryMutex.RUnlock()

	// Simulate streaming logs
	for {
		// Wait for a log message
		logMessage, ok := <-messages
		if !ok {
			return
		}

		// Write the log message to the websocket
		if err := writeLogMessage(ws, logMessage, location); err != nil {
			log.Error().Msgf("agent: error writing message: %s", err)
			return
		}
	}
}

func writeLogMessage(ws *websocket.Conn, logMessage *msg.LogMessage, location *time.Location) error {
	// Add the date and time in the users timezone
	prefix := "\033[90m" + logMessage.Date.In(location).Format("02 Jan 06 15:04:05 MST") + "\033[0m "

	switch logMessage.Source {
	case msg.MSG_LOG_SYSLOG:
		prefix = prefix + "\033[93mSYSLOG\033[0m "
	case msg.MSG_LOG_DBG:
		prefix = prefix + "\033[94mDBG\033[0m "
	case msg.MSG_LOG_INF:
		prefix = prefix + "\033[92mINF\033[0m "
	case msg.MSG_LOG_ERR:
		prefix = prefix + "\033[91mERR\033[0m "
	}

	return ws.WriteMessage(websocket.TextMessage, []byte(prefix+logMessage.Message+"\r\n"))
}
