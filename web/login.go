package web

import (
	"net/http"
	"net/url"
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

		data := map[string]interface{}{
			"redirect":            redirect,
			"version":             build.Version,
			"totpEnabled":         cfg.TOTP.Enabled,
			"logoURL":             cfg.UI.LogoURL,
			"logoInvert":          cfg.UI.LogoInvert,
			"pluginLogoLight":     pluginLogoLight(cfg),
			"pluginLogoDark":      pluginLogoDark(cfg),
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

	middleware.DeleteSessionCookie(w)

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
