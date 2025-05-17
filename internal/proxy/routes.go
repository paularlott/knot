package proxy

import (
	"net/http"

	"github.com/paularlott/knot/internal/middleware"

	"github.com/spf13/viper"
)

func Routes(router *http.ServeMux) {

	if viper.GetBool("server.enable_proxy") {
		router.HandleFunc("GET /proxy/port/{host}/{port}", middleware.ApiAuth(HandleWSProxyServer))
	}

	router.HandleFunc("GET /proxy/spaces/{space_id}/code-server/", middleware.ApiAuth(HandleSpacesCodeServerProxy))
	router.HandleFunc("GET /proxy/spaces/{space_id}/terminal/{shell}", middleware.ApiAuth(HandleSpacesTerminalProxy))

	router.HandleFunc("GET /proxy/spaces/{space_name}/port/{port}", middleware.ApiAuth(HandleSpacesPortProxy))
	router.HandleFunc("GET /proxy/spaces/{space_name}/ssh/", middleware.ApiAuth(HandleSpacesSSHProxy))
}

// Setup proxying of URLs to ports within spaces
func PortRoutes() *http.ServeMux {
	router := http.NewServeMux()
	router.HandleFunc("/", HandleSpacesWebPortProxy)
	return router
}
