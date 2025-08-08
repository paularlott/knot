package api

import (
	"net/http"
	"sort"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/rest"
)

func HandleGetClusterInfo(w http.ResponseWriter, r *http.Request) {
	peers := service.GetTransport().Nodes()
	response := make([]apiclient.ClusterNodeInfo, len(peers))

	for i, p := range peers {
		response[i] = apiclient.ClusterNodeInfo{
			Id:       p.ID.String(),
			Address:  p.GetAdvertisedAddress(),
			State:    p.GetState().String(),
			Metadata: p.Metadata.GetAllAsString(),
		}
	}
	// Sort the response by Address
	sort.Slice(response, func(i, j int) bool {
		return response[i].Address < response[j].Address
	})

	rest.WriteResponse(http.StatusOK, w, r, response)
}
