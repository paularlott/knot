package middleware

import (
	"net/http"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func GetSessionFromCookie(r *http.Request) (*model.Session, error) {
	// Get the cookie value
	cookie, err := GetCookie(r, model.WebSessionCookie)
	if err == nil {
		db := database.GetSessionStorage()
		session, err := db.GetSession(cookie.Value)
		return session, err
	}

	return nil, nil
}

// CookieSecure reports whether the web session cookie should carry the
// Secure attribute: knot terminates TLS itself, or the request arrived
// encrypted — directly (r.TLS) or behind a proxy that advertises the
// frontend scheme (X-Forwarded-Proto). A spoofed X-Forwarded-Proto can
// only ever add Secure, never remove it, so honouring the header is safe.
// It must stay conditional: plain-HTTP deployments (TLS terminated
// elsewhere or none) would have a Secure cookie rejected by the browser,
// breaking sessions — and a Secure deletion cookie is likewise ignored on
// plain HTTP, which would break logout.
func CookieSecure(r *http.Request, cfg *config.ServerConfig) bool {
	return cfg.TLS.UseTLS || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

func DeleteSessionCookie(w http.ResponseWriter, r *http.Request) {
	ExpireCookie(w, r, model.WebSessionCookie)

	// Earlier versions shared the cookie with wildcard subdomains; delete
	// one left from then so it can't shadow a fresh session or outlive a
	// logout.
	cfg := config.GetServerConfig()
	if domain := cfg.LegacySessionCookieDomain(); domain != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     model.WebSessionCookie,
			Path:     "/",
			Domain:   domain,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   CookieSecure(r, cfg),
			SameSite: http.SameSiteLaxMode,
		})
	}
}
