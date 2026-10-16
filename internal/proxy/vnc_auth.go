package proxy

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/crypt"
)

// Web VNC runs on its own host, "<owner>--<space>--vnc.<wildcard domain>",
// which knot's session cookie never reaches: the cookie stays on knot's
// own host, so the user code behind other wildcard hosts can't receive it.
// The VNC host signs a browser in with a handoff instead:
//
//  1. Without its cookie, the VNC host sends the browser to
//     /proxy/vnc-login on knot's host.
//  2. That page, behind the usual web sign-in, seals a single-use code for
//     the session, bound to the VNC host, and sends the browser back to
//     vncAuthPath on the VNC host with it.
//  3. The VNC host trades the code for vncGrantCookie, its own host-only
//     cookie, holding a grant for the same session.
//
// The grant is checked against the session on every request, so signing
// out, or a back-channel logout, ends VNC access with it.

const (
	// vncAuthPath is where the VNC host takes a handoff code. Requests to
	// it never reach the space.
	vncAuthPath = "/.knot-auth/vnc"

	vncGrantCookie = util.KnotCookiePrefix + "VNC"

	// vncHandoffLife is how long a handoff code works: it only has to
	// survive two redirects.
	vncHandoffLife = time.Minute
)

// usedVNCHandoffs keeps each handoff code's nonce until the code expires,
// so a code works once on this node.
var usedVNCHandoffs = &singleUse{}

type singleUse struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

// first records the key and reports whether this is its first use.
func (s *singleUse) first(key string, forgetAt, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, t := range s.seen {
		if now.After(t) {
			delete(s.seen, k)
		}
	}
	if _, ok := s.seen[key]; ok {
		return false
	}
	if s.seen == nil {
		s.seen = map[string]time.Time{}
	}
	s.seen[key] = forgetAt
	return true
}

// handleVNCHost authenticates a request to a VNC host, then proxies it.
func handleVNCHost(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetServerConfig()

	if r.URL.Path == vncAuthPath {
		handleVNCHandoff(w, r, cfg)
		return
	}

	// API tokens authenticate as they always have.
	if r.Header.Get("Authorization") != "" {
		middleware.ApiAuth(HandleSpacesWebPortProxy)(w, r)
		return
	}

	if c, err := middleware.GetCookie(r, vncGrantCookie); err == nil {
		g, ok := crypt.OpenVNCGrant(cfg.EncryptionKey, r.Host, c.Value)
		if ok && (g.Kind != crypt.VNCGrantCookie || !liveSession(g.SessionId)) {
			// Signed out since: drop the cookie and sign in again.
			ok = false
			middleware.ExpireCookie(w, r, vncGrantCookie)
		}
		if ok {
			// Authenticate as the grant's session, exactly as knot's own
			// host would: ApiAuth checks it is live, extends it, and loads
			// its user. The proxy strips knot's cookies before the space.
			util.StripKnotCookies(r.Header)
			r.AddCookie(&http.Cookie{Name: middleware.CookieName(model.WebSessionCookie), Value: g.SessionId})
			middleware.ApiAuth(HandleSpacesWebPortProxy)(w, r)
			return
		}
	}

	// A browser opening the VNC host is sent to sign in; anything else,
	// such as the VNC client's websocket, is just refused.
	if r.Method == http.MethodGet && !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		login := url.Values{"host": {r.Host}}
		if back := util.LocalRedirect(r.URL.RequestURI()); back != "" {
			login.Set("return", back)
		}
		http.Redirect(w, r, strings.TrimRight(cfg.URL, "/")+"/proxy/vnc-login?"+login.Encode(), http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
}

// HandleVNCLogin, on knot's own host behind WebAuth, sends the browser
// back to the VNC host with a single-use code for its session.
func HandleVNCLogin(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetServerConfig()
	session, ok := r.Context().Value("session").(*model.Session)
	if !ok || session == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	host := r.URL.Query().Get("host")
	if !isWildcardVNCHost(cfg, host) {
		http.Error(w, "not a VNC host", http.StatusBadRequest)
		return
	}

	code, err := crypt.SealVNCGrant(cfg.EncryptionKey, host, crypt.VNCGrant{
		Kind:      crypt.VNCGrantHandoff,
		SessionId: session.Id,
		Expires:   time.Now().Add(vncHandoffLife).Unix(),
	})
	if err != nil {
		log.Error("vnc login: failed to seal handoff", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	scheme := "https"
	if u, err := url.Parse(cfg.URL); err == nil && u.Scheme != "" {
		scheme = u.Scheme
	}
	q := url.Values{"code": {code}}
	if back := util.LocalRedirect(r.URL.Query().Get("return")); back != "" {
		q.Set("return", back)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, scheme+"://"+host+vncAuthPath+"?"+q.Encode(), http.StatusSeeOther)
}

// handleVNCHandoff, on the VNC host, trades a handoff code for the VNC
// host's own cookie.
func handleVNCHandoff(w http.ResponseWriter, r *http.Request, cfg *config.ServerConfig) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")

	now := time.Now()
	g, ok := crypt.OpenVNCGrant(cfg.EncryptionKey, r.Host, r.URL.Query().Get("code"))
	if !ok || g.Kind != crypt.VNCGrantHandoff || now.After(time.Unix(g.Expires, 0)) {
		http.Error(w, "VNC sign-in link is invalid or has expired", http.StatusBadRequest)
		return
	}
	if !usedVNCHandoffs.first(g.Nonce, time.Unix(g.Expires, 0).Add(time.Minute), now) {
		http.Error(w, "VNC sign-in link has already been used", http.StatusBadRequest)
		return
	}

	if !liveSession(g.SessionId) {
		http.Error(w, "VNC sign-in link is invalid or has expired", http.StatusBadRequest)
		return
	}

	grant, err := crypt.SealVNCGrant(cfg.EncryptionKey, r.Host, crypt.VNCGrant{
		Kind:      crypt.VNCGrantCookie,
		SessionId: g.SessionId,
	})
	if err != nil {
		log.Error("vnc login: failed to seal grant", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Host-only: no Domain, so it never reaches any other host.
	middleware.SetCookie(w, r, vncGrantCookie, grant, 0)

	back := util.LocalRedirect(r.URL.Query().Get("return"))
	if back == "" || strings.HasPrefix(back, vncAuthPath) {
		back = "/"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// liveSession reports whether a web session is still signed in.
func liveSession(id string) bool {
	session, err := database.GetSessionStorage().GetSession(id)
	return err == nil && session != nil && !session.IsDeleted && session.ExpiresAfter.After(time.Now().UTC())
}

// isWildcardVNCHost reports whether host is a VNC host under the server's
// wildcard domain, so knot never mints a handoff for, or redirects to, any
// other host.
func isWildcardVNCHost(cfg *config.ServerConfig, host string) bool {
	if host == "" || !isVNCSubdomain(host) {
		return false
	}
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	domain := strings.TrimPrefix(strings.TrimPrefix(cfg.WildcardDomain, "*"), ".")
	label, rest, _ := strings.Cut(name, ".")
	return domain != "" && label != "" && strings.EqualFold(rest, domain) && !strings.ContainsAny(host, "/\\@?#")
}
