package web

import (
	"net/http"

	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/rest"

	"github.com/gorilla/mux"
	"github.com/spf13/viper"
)

type LookupResponse struct {
  Status bool `json:"status"`
  Host string `json:"host"`
  Port string `json:"port"`
}

func HandleLookup(w http.ResponseWriter, r *http.Request) {
  var host string
  var port string
  var err error

  vars := mux.Vars(r)
  service := vars["service"]

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
