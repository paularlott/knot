package web

import (
	"encoding/json"
	"net/http"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
)

func HandleGetIcons(w http.ResponseWriter, r *http.Request) {
	iconService := service.GetIconService()
	icons := iconService.GetIcons()

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(icons)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
	}
}
