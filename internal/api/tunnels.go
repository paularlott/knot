package api

import (
	"net/http"
	"sort"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/tunnel_server"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/spf13/viper"
)

func HandleGetTunnels(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	tunnels := tunnel_server.GetTunnelsForUser(user.Id)

	sort.Strings(tunnels)

	tunnelList := make([]apiclient.TunnelInfo, len(tunnels))
	for i, tunnel := range tunnels {
		tunnelList[i] = apiclient.TunnelInfo{
			Name:    user.Username + "--" + tunnel,
			Address: "https://" + user.Username + "--" + tunnel + viper.GetString("server.tunnel_domain"),
		}
	}

	rest.SendJSON(http.StatusOK, w, r, tunnelList)
}

func HandleGetTunnelDomain(w http.ResponseWriter, r *http.Request) {
	rest.SendJSON(http.StatusOK, w, r, viper.GetString("server.tunnel_domain"))
}

func HandleDeleteTunnel(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	tunnelName := r.PathValue("tunnel_name")
	if tunnelName == "" {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "tunnel_name parameter is required"})
		return
	}

	err := tunnel_server.DeleteTunnel(user.Id, tunnelName)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	} else {
		rest.SendJSON(http.StatusOK, w, r, ErrorResponse{Error: "tunnel deleted"})
	}
}
