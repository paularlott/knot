package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/util/rest"
)

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
