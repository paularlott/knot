package api

import (
	"net/http"

	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/rest"
)

func HandleGetIcons(w http.ResponseWriter, r *http.Request) {
	iconService := service.GetIconService()
	icons := iconService.GetIcons()

	rest.WriteResponse(http.StatusOK, w, r, icons)
}
