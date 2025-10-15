package proxy

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/knot/internal/log"
)

func HandleSpacesPortProxy(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceName := r.PathValue("space_name")
	if !validate.Name(spaceName) {
		log.Debug("Invalid space name", "space_name", spaceName)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	port := r.PathValue("port")
	portUInt, err := strconv.ParseUint(port, 10, 16)
	if err != nil || !validate.IsNumber(int(portUInt), 0, 65535) {
		log.Debug("Invalid port", "port", port)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpaceByName(user.Id, spaceName)
	if err != nil {
		log.Error("Error loading space", "error", err, "space_name", spaceName)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the space session
	agentSession := agent_server.GetSession(space.Id)
	if agentSession == nil {
		log.Debug("Space session not found", "space_name", spaceName)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	/* 	// Check the port is allowed must be in the TcpPorts or HttpPorts
	   	_, tcpOk := agentSession.TcpPorts[port]
	   	_, httpOk := agentSession.HttpPorts[port]
	   	if !tcpOk && !httpOk {
	   		log.Debug("Port not allowed", "port", port)
	   		w.WriteHeader(http.StatusNotFound)
	   		return
	   	} */

	proxyAgentPort(w, r, agentSession, uint16(portUInt))
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
	if agentSession == nil || (domainParts[2] == "vnc" && (agentSession.VNCHttpPort == 0 || !user.HasPermission(model.PermissionUseVNC))) {
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
		if err := msg.WriteCommand(stream, msg.CmdProxyVNC); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		tokenStr := "Basic " + base64.StdEncoding.EncodeToString([]byte("knot:"+user.ServicePassword))
		token = &tokenStr
	} else {
		if err := msg.WriteCommand(stream, msg.CmdProxyHTTP); err != nil {
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

	proxy := CreateAgentReverseProxy(targetURL, stream, token, r.Host)
	proxy.ServeHTTP(w, r)
}
