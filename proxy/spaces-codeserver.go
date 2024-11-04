package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/go-chi/chi/v5"
)

func HandleSpacesCodeServerProxy(w http.ResponseWriter, r *http.Request) {
	spaceId := chi.URLParam(r, "space_id")
	user := r.Context().Value("user").(*model.User)

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Check user access to the space
	if space.UserId != user.Id {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the space session
	agentSession := agent_server.GetSession(space.Id)
	if agentSession == nil || !agentSession.HasCodeServer {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Open a new stream to the agent
	stream, err := agentSession.MuxSession.Open()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	// Write the terminal command
	err = msg.WriteCommand(stream, msg.MSG_CODE_SERVER)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	r.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/proxy/spaces/%s/code-server", spaceId))

	targetURL, err := url.Parse("http://127.0.0.1/")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	proxy := createAgentReverseProxy(targetURL, stream, nil)
	proxy.ServeHTTP(w, r)
}
