package api

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/util/rest"
)

func HandleGetClusterInfo(w http.ResponseWriter, r *http.Request) {
	peers := service.GetTransport().Nodes()
	response := make([]apiclient.ClusterNodeInfo, len(peers))

	for i, p := range peers {
		response[i] = apiclient.ClusterNodeInfo{
			Id:       p.ID.String(),
			Address:  p.GetAddress().String(),
			State:    p.GetState().String(),
			Metadata: p.Metadata.GetAllAsString(),
		}
	}

	rest.SendJSON(http.StatusOK, w, r, response)
}
