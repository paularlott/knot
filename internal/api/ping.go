package api

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util/rest"
)

func HandlePing(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetServerConfig()
	rest.WriteResponse(http.StatusOK, w, r, apiclient.PingResponse{
		Status:  true,
		Version: build.Version,
		Zone:    cfg.Zone,
	})
}
