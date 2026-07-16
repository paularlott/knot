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
	"github.com/paularlott/knot/internal/service"
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

	db := database.GetInstance()

	// Load the space — fall back to pool routing if not found
	space, err := db.GetSpaceByName(user.Id, spaceName)
	if err != nil || space == nil {
		space = service.GetPoolService().PickMemberForRouting(spaceName, user.Id)
		if space == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}

	agentSession := agent_server.GetSession(space.Id)
	if agentSession == nil {
		log.Debug("Space session not found", "space_name", spaceName)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	proxyAgentPort(w, r, agentSession, uint16(portUInt))
}

// Proxy a web port for a space or pool, the transport is http and the agent
// works out the http / https connection.
func HandleSpacesWebPortProxy(w http.ResponseWriter, r *http.Request) {
	var token *string = nil

	domainParts := strings.Split(r.Host, ".")
	if len(domainParts) < 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	domainParts = strings.Split(domainParts[0], "--")
	if len(domainParts) != 3 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// VNC subdomains are auth-gated by PortRoutes and resolved against the
	// authenticated viewer so that shared spaces work. Web ports stay open.
	if domainParts[2] == "vnc" {
		handleVNCProxy(w, r, domainParts)
		return
	}

	db := database.GetInstance()

	user, err := db.GetUserByUsername(domainParts[0])
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Load the space — fall back to pool routing if not found
	space, err := db.GetSpaceByName(user.Id, domainParts[1])
	if err != nil || space == nil {
		space = service.GetPoolService().PickMemberForRouting(domainParts[1], user.Id)
		if space == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}

	agentSession := agent_server.GetSession(space.Id)
	if agentSession == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if _, ok := agentSession.HttpPorts[domainParts[2]]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	stream, err := agentSession.MuxSession.Open()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer stream.Close()

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

	targetURL, err := url.Parse("http://127.0.0.1/")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	proxy := CreateAgentReverseProxy(targetURL, stream, token, r.Host)
	proxy.ServeHTTP(w, r)
}

// handleVNCProxy authenticates the viewer (PortRoutes gates the VNC subdomain
// behind ApiAuth) and proxies the request to the space's web VNC port. Access
// is granted to the space owner and any user the space is shared with, matching
// the SSH and terminal proxies. The VNC server inside the space authenticates
// against the space owner's service password.
func handleVNCProxy(w http.ResponseWriter, r *http.Request, domainParts []string) {
	viewer, ok := r.Context().Value("user").(*model.User)
	if !ok || viewer == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if !viewer.HasPermission(model.PermissionUseVNC) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	db := database.GetInstance()

	// Resolve the space by name for the viewer; GetSpaceByName considers spaces
	// owned by the viewer as well as spaces shared with them.
	space, err := db.GetSpaceByName(viewer.Id, domainParts[1])
	if err != nil || space == nil {
		space = service.GetPoolService().PickMemberForRouting(domainParts[1], viewer.Id)
		if space == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}

	// Explicit access check, matching the terminal proxy.
	if space.UserId != viewer.Id && !space.IsSharedWith(viewer.Id) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	agentSession := agent_server.GetSession(space.Id)
	if agentSession == nil || agentSession.VNCHttpPort == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	stream, err := agentSession.MuxSession.Open()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	if err := msg.WriteCommand(stream, msg.CmdProxyVNC); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// The VNC server authenticates against the space owner's service password,
	// so resolve the owner (not the viewer) for the auth token.
	owner, err := db.GetUser(space.UserId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	tokenStr := "Basic " + base64.StdEncoding.EncodeToString([]byte("knot:"+owner.ServicePassword))

	targetURL, err := url.Parse("http://127.0.0.1/")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	proxy := CreateAgentReverseProxy(targetURL, stream, &tokenStr, r.Host)
	proxy.ServeHTTP(w, r)
}
