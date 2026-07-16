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
	// Clear both cookie scopes so a stale cookie from before a domain change
	// (or a host-only cookie left over from before wildcard widening) can't
	// shadow a freshly issued session. The host-only deletion always runs; the
	// domain-scoped deletion only runs when the session cookie is widened.
	expireSessionCookie(w, "", cfg)
	if domain := cfg.SessionCookieDomain(); domain != "" {
		expireSessionCookie(w, domain, cfg)
	}
}

func expireSessionCookie(w http.ResponseWriter, domain string, cfg *config.ServerConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     model.WebSessionCookie,
		Value:    "",
		Path:     "/",
		Domain:   domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.TLS.UseTLS,
		SameSite: http.SameSiteLaxMode,
	})
}
