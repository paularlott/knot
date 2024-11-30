package proxy

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/go-chi/chi/v5"
)

func HandleSpacesPortProxy(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceName := chi.URLParam(r, "space_name")
	port := chi.URLParam(r, "port")

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpaceByName(user.Id, spaceName)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the space session
	agentSession := agent_server.GetSession(space.Id)
	if agentSession == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Check the port is allowed
	if _, ok := agentSession.TcpPorts[port]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	portInt, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	proxyAgentPort(w, r, agentSession, uint16(portInt))
}

// Proxy a web port for a space or VNC, the transport is http and the agent works out the http / https connection
func HandleSpacesWebPortProxy(w http.ResponseWriter, r *http.Request) {
	var token *string = nil

	// Split the domain into parts
	domainParts := strings.Split(r.Host, ".")
	if len(domainParts) < 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Extract the user, space and port from the domain
	domainParts = strings.Split(domainParts[0], "--")
	if len(domainParts) != 3 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	db := database.GetInstance()

	// Load the user
	user, err := db.GetUserByUsername(domainParts[0])
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Load the space
	space, err := db.GetSpaceByName(user.Id, domainParts[1])
	if err != nil || space == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the space session
	agentSession := agent_server.GetSession(space.Id)
	if agentSession == nil || (domainParts[2] == "vnc" && agentSession.VNCHttpPort == 0) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// If not VNC then check the port is allowed
	if domainParts[2] != "vnc" {
		if _, ok := agentSession.HttpPorts[domainParts[2]]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}

	// Open a new stream to the agent
	stream, err := agentSession.MuxSession.Open()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	// Write the command
	if domainParts[2] == "vnc" {
		if err := msg.WriteCommand(stream, msg.MSG_PROXY_VNC); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		tokenStr := "Basic " + base64.StdEncoding.EncodeToString([]byte("knot:"+user.ServicePassword))
		token = &tokenStr
	} else {
		if err := msg.WriteCommand(stream, msg.MSG_PROXY_HTTP); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		port, err := strconv.ParseUint(domainParts[2], 10, 16)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err := msg.WriteMessage(stream, &msg.HttpPort{
			Port:       uint16(port),
			ServerName: r.Host,
		}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	targetURL, err := url.Parse("http://127.0.0.1/")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	proxy := createAgentReverseProxy(targetURL, stream, token)
	proxy.ServeHTTP(w, r)
}
