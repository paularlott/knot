package apiv1

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/rest"
	"github.com/rs/zerolog/log"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/viper"
)

/**
* @api {get} /api/v1/lookup/:service Lookup
* @apiVersion 1.0.0
* @apiName Lookup
* @apiGroup General
* @apiDescription Lookup a service via DNS SRV or A record.
*
* @apiParam {String} service The name of the service to lookup
*
* @apiSuccess {Boolean} status True if the service was found
* @apiSuccess {String} host The host of the service
* @apiSuccess {String} port The port of the service
*
* @apiSuccessExample Success-Response:
*     HTTP/1.1 200 OK
*     {
*       "status": true,
*       "host": "example.com",
*       "port": "80"
*     }
 */

type LookupResponse struct {
  Status bool `json:"status"`
  Host string `json:"host"`
  Port string `json:"port"`
}

func HandleLookup(w http.ResponseWriter, r *http.Request) {
  var host string
  var port string
  var err error

  service := chi.URLParam(r, "service")

  log.Debug().Msgf("Looking up %s", service)

  response := LookupResponse{Status: true, Host: "", Port: ""}

  host, port, err = util.GetTargetFromSRV(service, viper.GetString("nameserver"))
  if err != nil {
    host, err = util.GetIP(service, viper.GetString("nameserver"))
    if err != nil {
      response.Status = false
    }
  }

  if response.Status {
    response.Host = host
    response.Port = port
  }

  w.WriteHeader(http.StatusOK)
  rest.SendJSON(w, response)
}

func CallLookup(client *rest.RESTClient, service string) (LookupResponse, error) {
  lookup := LookupResponse{}
  err := client.Get(fmt.Sprintf("/api/v1/lookup/%s", service), &lookup)
  return lookup, err
}

