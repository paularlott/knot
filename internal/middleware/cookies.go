package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/paularlott/knot/internal/config"
)

// hostCookiePrefix makes a browser accept a cookie only from the exact
// host that sets it, Secure, with Path=/ and no Domain. Without it, a web
// server on a wildcard host, which runs a user's own code, could set a
// cookie for the parent domain that the browser then sends knot: a session
// or sign-in cookie of the attacker's, signing the victim in as them.
const hostCookiePrefix = "__Host-"

// HostCookies reports whether knot's cookies take the __Host- prefix. It
// follows the server's public URL rather than the request, so every server
// in a zone behind a load balancer that ends TLS agrees on the name, with
// or without X-Forwarded-Proto. The prefix needs Secure, which a browser
// only keeps over https; over plain http the names stay unprefixed.
func HostCookies(cfg *config.ServerConfig) bool {
	u, err := url.Parse(cfg.URL)
	return err == nil && strings.EqualFold(u.Scheme, "https")
}

// CookieName is the name knot uses for one of its cookies.
func CookieName(name string) string {
	if HostCookies(config.GetServerConfig()) {
		return hostCookiePrefix + name
	}
	return name
}

// GetCookie reads one of knot's cookies. On an https server only the
// prefixed name counts, so a cookie planted under the plain name is
// ignored.
func GetCookie(r *http.Request, name string) (*http.Cookie, error) {
	return r.Cookie(CookieName(name))
}

// SetCookie sets one of knot's cookies, host-only and HttpOnly. maxAge 0
// makes it last as long as the browser session.
func SetCookie(w http.ResponseWriter, r *http.Request, name, value string, maxAge int) {
	cfg := config.GetServerConfig()
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName(name),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   HostCookies(cfg) || CookieSecure(r, cfg),
		SameSite: http.SameSiteLaxMode,
	})
}

// ExpireCookie deletes one of knot's cookies under both names, so one left
// from before the server's URL changed scheme goes too.
func ExpireCookie(w http.ResponseWriter, r *http.Request, name string) {
	cfg := config.GetServerConfig()
	for _, n := range []string{hostCookiePrefix + name, name} {
		http.SetCookie(w, &http.Cookie{
			Name:     n,
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			// A __Host- cookie can only be deleted with Secure.
			Secure:   strings.HasPrefix(n, hostCookiePrefix) || CookieSecure(r, cfg),
			SameSite: http.SameSiteLaxMode,
		})
	}
}
