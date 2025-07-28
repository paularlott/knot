package middleware

import (
	"net/http"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func GetSessionFromCookie(r *http.Request) *model.Session {
	var session *model.Session = nil

	// Get the cookie value
	cookie, err := r.Cookie(model.WebSessionCookie)
	if err == nil {
		db := database.GetSessionStorage()
		session, _ = db.GetSession(cookie.Value)
	} else {
		key := r.Header.Get(model.WebSessionCookie)
		if key != "" {
			db := database.GetSessionStorage()
			session, _ = db.GetSession(key)
		}
	}

	return session
}

func DeleteSessionCookie(w http.ResponseWriter) {
	cfg := config.GetServerConfig()
	http.SetCookie(w, &http.Cookie{
		Name:     model.WebSessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.TLS.UseTLS,
		SameSite: http.SameSiteLaxMode,
	})
}
