package web

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/audit"

	"github.com/paularlott/knot/internal/log"
)

// defaultLoginPage is the post-login landing page: a plugin page claimed
// with default = true when one exists (first plugin by name), else /spaces.
func defaultLoginPage() string {
	if registry := plugins.GetRegistry(); registry != nil {
		if url := registry.DefaultPageURL(); url != "" {
			return url
		}
	}
	return "/spaces"
}

func HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetServerConfig()

	if !middleware.HasUsers && cfg.Origin.Server == "" && cfg.Origin.Token == "" {
		http.Redirect(w, r, "/initial-system-setup", http.StatusSeeOther)
	} else {
		session, _ := middleware.GetSessionFromCookie(r)

		// If session present then redirect to the landing page (a plugin
		// page when one claims default, else /spaces)
		if session != nil {
			http.Redirect(w, r, defaultLoginPage(), http.StatusSeeOther)
			return
		}

		tmpl, err := newTemplate("login.tmpl")
		if err != nil {
			log.Error(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Parse the URL to redirect to to get just the path
		var redirect string
		redirectParam := r.URL.Query().Get("redirect")
		u, _ := url.Parse(redirectParam)
		if u.Path == "" || u.Path == "/logout" {
			redirect = defaultLoginPage()
		} else if u.Path[0:1] != "/" {
			redirect = "/" + u.Path
		} else {
			// Preserve both path and query parameters for OAuth redirects
			if u.RawQuery != "" {
				redirect = u.Path + "?" + u.RawQuery
			} else {
				redirect = u.Path
			}
		}

		pluginLogoLight, pluginLogoDark := pluginLogoURLs(cfg)
		data := map[string]interface{}{
			"redirect":            redirect,
			"version":             build.Version,
			"assetVersion":        assetVersionKey(),
			"totpEnabled":         cfg.TOTP.Enabled,
			"logoURL":             cfg.UI.LogoURL,
			"logoInvert":          cfg.UI.LogoInvert,
			"pluginLogoLight":     pluginLogoLight,
			"pluginLogoDark":      pluginLogoDark,
			"passwordAuthEnabled": true,
		}

		err = tmpl.Execute(w, data)
		if err != nil {
			log.Error(err.Error())
		}
	}
}

func HandleLogoutPage(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value("session").(*model.Session)
	if session != nil {
		session.IsDeleted = true
		session.ExpiresAfter = time.Now().Add(model.SessionExpiryDuration).UTC()
		session.UpdatedAt = hlc.Now()
		database.GetSessionStorage().SaveSession(session)
		service.GetTransport().GossipSession(session)
		sse.GetHub().InvalidateSession(session.Id)
	}

	middleware.DeleteSessionCookie(w, r)

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// HandleSwitchUserPage moves the current web session onto another user —
// fast user switching, one way. The become-list is maintained by the user
// manager (PermissionLinkUsers): a user may switch to any account it is
// linked to, never the reverse. The one exception is flicking back: the
// session may always return to the account that originally authenticated
// (OriginalUserId), which grants the target nothing.
//
// Called by fetch from the profile menu, so it answers with a bare status
// — never a redirect. A redirect would make fetch follow it and render the
// whole landing page into the discarded response body, doubling the work
// and whitening the screen before the real navigation starts.
func HandleSwitchUserPage(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	session := r.Context().Value("session").(*model.Session)

	target, err := database.GetInstance().GetUser(r.PathValue("user_id"))
	if err != nil || target == nil || target.IsDeleted || !target.Active || session == nil {
		deniedSwitchAudit(r, user, r.PathValue("user_id"), "user not available for switching")
		http.Error(w, "user not available for switching", http.StatusForbidden)
		return
	}
	became := slices.Contains(user.LinkedUsers, target.Id)
	flickBack := session.OriginalUserId != "" && target.Id == session.OriginalUserId
	if !became && !flickBack {
		deniedSwitchAudit(r, user, target.Id, "target not in the user's become-list")
		http.Error(w, "user not available for switching", http.StatusForbidden)
		return
	}

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventUserSwitch,
		fmt.Sprintf("Switched session to user %s", target.Username),
		&map[string]interface{}{
			"from_user_id": user.Id,
			"from_user":    user.Username,
			"to_user_id":   target.Id,
			"to_user":      target.Username,
			"flick_back":   flickBack,
		},
	)

	session.UserId = target.Id
	// First switch stamps the origin; flicking back and forth keeps it.
	if session.OriginalUserId == "" {
		session.OriginalUserId = user.Id
	}
	session.UpdatedAt = hlc.Now()
	database.GetSessionStorage().SaveSession(session)
	service.GetTransport().GossipSession(session)
	// Drop the SSE streams bound to the old identity so they reconnect as
	// the target user — WITHOUT the auth:required signal InvalidateSession
	// uses: the client answers that by navigating to /logout, which deletes
	// the just-switched session (and races the switcher's own navigation to
	// /, so which one won was random).
	sse.GetHub().CloseSession(session.Id)

	w.WriteHeader(http.StatusNoContent)
}

// deniedSwitchAudit records a refused switch — probing ids, stale menus,
// forged requests — so a burst of attempts from one session is visible in
// the audit trail like failed logins are.
func deniedSwitchAudit(r *http.Request, user *model.User, targetId, reason string) {
	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventUserSwitchDenied,
		fmt.Sprintf("Switch to user refused: %s", reason),
		&map[string]interface{}{
			"from_user_id":   user.Id,
			"requested_user": targetId,
			"reason":         reason,
		},
	)
}
