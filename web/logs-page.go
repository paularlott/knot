package web

import (
	"net/http"
	"time"
	_ "time/tzdata"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/validate"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func HandleLogsPage(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

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
	if space.UserId != user.Id && space.SharedWithUserId != user.Id {
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
	user := r.Context().Value("user").(*model.User)

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
		ws.Close()
		return
	}

	// Register a notification channel with the session
	listenerId, channel := agentSession.RegisterLogListener()
	if channel == nil {
		w.WriteHeader(http.StatusInternalServerError)
		ws.Close()
		return
	}

	// Monitor for the websocket closing
	go func() {
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				log.Debug().Msgf("websocket closed: %s", err)
				agentSession.UnregisterLogListener(listenerId)
				return
			}
		}
	}()

	// Write the log history to the websocket
	agentSession.LogHistoryMutex.RLock()
	for _, logMessage := range agentSession.LogHistory {
		if err := writeLogMessage(ws, logMessage, location); err != nil {
			log.Error().Msgf("agent: error writing message: %s", err)
			agentSession.LogHistoryMutex.RUnlock()
			return
		}
	}
	agentSession.LogHistoryMutex.RUnlock()

	// Send a marker to indicate the end of the history
	ws.WriteMessage(websocket.TextMessage, []byte{0})

	// Simulate streaming logs
	for {
		// Wait for a log message
		logMessage, ok := <-channel
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
