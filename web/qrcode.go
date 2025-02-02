package web

import (
	"net/http"

	"github.com/paularlott/knot/internal/totp"
)

func HandleCreateQRCode(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	// TODO add option to enable TOTP and set the TOTP application name
	err := totp.ServeCreateQRCode(w, code, "knot")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
