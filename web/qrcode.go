package web

import (
	"net/http"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/totp"
)

func HandleCreateQRCode(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	cfg := config.GetServerConfig()
	err := totp.ServeCreateQRCode(w, code, cfg.TOTP.Issuer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}
