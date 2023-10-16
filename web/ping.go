package web

import (
	"encoding/json"
	"net/http"

	"github.com/paularlott/knot/build"
)

type PingResponse struct {
  Status bool `json:"status"`
  Version string `json:"version"`
}

func HandlePing(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json")
  w.WriteHeader(http.StatusOK)

  response := PingResponse{
    Status: true,
    Version: build.Version + " (" + build.Date + ")",
  }
  json.NewEncoder(w).Encode(response)
}
