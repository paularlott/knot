package web

import (
	"net/http"

	"github.com/paularlott/knot/internal/totp"

	"github.com/spf13/viper"
)

func HandleCreateQRCode(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	err := totp.ServeCreateQRCode(w, code, viper.GetString("server.totp.issuer"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}
