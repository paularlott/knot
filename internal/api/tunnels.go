package api

import (
	"net/http"
	"sort"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/tunnel_server"
	"github.com/paularlott/knot/internal/util/rest"
)

func HandleGetTunnels(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	tunnels := tunnel_server.GetTunnelsForUser(user.Id)
	cfg := config.GetServerConfig()

	sort.Strings(tunnels)

	tunnelList := make([]apiclient.TunnelInfo, len(tunnels))
	for i, tunnel := range tunnels {
		tunnelList[i] = apiclient.TunnelInfo{
			Name:    user.Username + "--" + tunnel,
			Address: "https://" + user.Username + "--" + tunnel + cfg.TunnelDomain,
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, tunnelList)
}

func HandleGetTunnelServerInfo(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetServerConfig()
	info := &apiclient.TunnelServerInfo{
		Domain:        cfg.TunnelDomain,
		TunnelServers: service.GetTransport().GetTunnelServers(),
	}

	rest.WriteResponse(http.StatusOK, w, r, info)
}

func HandleDeleteTunnel(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	tunnelName := r.PathValue("tunnel_name")
	if tunnelName == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "tunnel_name parameter is required"})
		return
	}

	err := tunnel_server.DeleteTunnel(user.Id, tunnelName)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	} else {
		rest.WriteResponse(http.StatusOK, w, r, ErrorResponse{Error: "tunnel deleted"})
	}
}
