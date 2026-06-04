package middleware

import (
	"net/http"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func GetSessionFromCookie(r *http.Request) (*model.Session, error) {
	// Get the cookie value
	cookie, err := r.Cookie(model.WebSessionCookie)
	if err == nil {
		db := database.GetSessionStorage()
		session, err := db.GetSession(cookie.Value)
		return session, err
	}

	key := r.Header.Get(model.WebSessionCookie)
	if key != "" {
		db := database.GetSessionStorage()
		session, err := db.GetSession(key)
		return session, err
	}

	return nil, nil
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
