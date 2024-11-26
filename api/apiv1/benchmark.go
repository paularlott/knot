package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/util/rest"
)

func HandleBenchmark(w http.ResponseWriter, r *http.Request) {
	var payload apiclient.BenchmarkPacket

	if err := rest.BindJSON(w, r, &payload); err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	rest.SendJSON(http.StatusOK, w, r, payload)
}
