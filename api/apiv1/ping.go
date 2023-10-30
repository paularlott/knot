package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/util/rest"
)

/**
* @api {get} /api/v1/ping Ping
* @apiVersion 1.0.0
* @apiName Ping
* @apiGroup General
* @apiDescription Ping the server and get a health response.
*
* @apiSuccess {Boolean} status True if the server is healthy
* @apiSuccess {String} version The version string
*
* @apiSuccessExample Success-Response:
*     HTTP/1.1 200 OK
*     {
*       "status": true,
*       "version": "1.0.0 (20231030.230326+0800)"
*     }
 */

type PingResponse struct {
  Status bool `json:"status"`
  Version string `json:"version"`
}

func HandlePing(w http.ResponseWriter, r *http.Request) {
  w.WriteHeader(http.StatusOK)
  rest.SendJSON(w, PingResponse{
    Status: true,
    Version: build.Version + " (" + build.Date + ")",
  })
}

func CallPing(client *rest.RESTClient) (PingResponse, error) {
  ping := PingResponse{}
  err := client.Get("/api/v1/ping", &ping)
  return ping, err
}
