package apiv1

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
	"github.com/rs/zerolog/log"
)

type LookupResponse struct {
	Status bool   `json:"status"`
	Host   string `json:"host"`
	Port   string `json:"port"`
}

func HandleLookup(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")

	if !validate.Subdomain(service) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid service name"})
		return
	}

	log.Debug().Msgf("lookup: looking up %s", service)

	response := LookupResponse{Status: true, Host: "", Port: ""}

	hostPort, err := util.LookupSRV(service)
	if err != nil {
		ips, err := util.LookupIP(service)
		if err != nil {
			response.Status = false
		} else {
			response.Host = (*ips)[0]
		}
	} else {
		response.Host = (*hostPort)[0].Host
		response.Port = (*hostPort)[0].Port
	}

	rest.SendJSON(http.StatusOK, w, r, response)
}

func CallLookup(client *rest.RESTClient, service string) (*LookupResponse, int, error) {
	lookup := &LookupResponse{}
	statusCode, err := client.Get(fmt.Sprintf("/api/lookup/%s", service), lookup)
	return lookup, statusCode, err
}
