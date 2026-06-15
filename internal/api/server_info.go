package api

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util/rest"
)

// HandleGetServerInfo returns server-wide configuration that clients need but
// that is not user-specific (e.g. the wildcard domain used to build space
// web-port URLs).
func HandleGetServerInfo(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetServerConfig()
	rest.WriteResponse(http.StatusOK, w, r, &apiclient.ServerInfoResponse{
		Version:        build.Version,
		WildcardDomain: cfg.WildcardDomain,
	})
}
