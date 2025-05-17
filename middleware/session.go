package middleware

import (
	"net/http"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/spf13/viper"
)

func GetSessionFromCookie(r *http.Request) *model.Session {
	var session *model.Session = nil

	// Get the cookie value
	cookie, err := r.Cookie(model.WEBUI_SESSION_COOKIE)
	if err == nil {

		// Load the session from the database
		db := database.GetSessionStorage()
		session, _ = db.GetSession(cookie.Value)
	}

	return session
}

func DeleteSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     model.WEBUI_SESSION_COOKIE,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   viper.GetBool("server.tls.use_tls"),
		SameSite: http.SameSiteLaxMode,
	})
}
