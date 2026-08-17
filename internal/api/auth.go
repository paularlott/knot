package api

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/authratelimit"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/totp"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleAuthorization(w http.ResponseWriter, r *http.Request) {
	var userId string = ""
	var showTOTPSecret string = ""

	db := database.GetInstance()
	request := apiclient.AuthLoginRequest{}

	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Get client IP (consistent with how we got it for rate limiting).
	// Normalized the same way as RequestProperties: first X-Forwarded-For
	// entry, port stripped — otherwise direct connections key the limiter
	// per TCP connection and never trip.
	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}
	if strings.Contains(clientIP, ",") {
		clientIP = strings.TrimSpace(strings.Split(clientIP, ",")[0])
	}
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	}

	cfg := config.GetServerConfig()

	if cfg.AuthIPRateLimiting {
		// Block auth while the IP or the account is rate limited
		if authratelimit.Blocked(clientIP, request.Email) {
			log.Warn("Rate limit exceeded for IP:", "clientIP", clientIP, "rate", request.Email)
			// Evidence of attempts while blocked (rate-limit evasion) belongs
			// in the audit trail and feeds anomaly detection.
			audit.LogWithRequest(r,
				request.Email,
				model.AuditActorTypeUser,
				model.AuditEventAuthBlocked,
				fmt.Sprintf("Blocked login attempt for %s", request.Email),
				&map[string]interface{}{
					"email": request.Email,
				},
			)
			rest.WriteResponse(http.StatusTooManyRequests, w, r, ErrorResponse{Error: "too many requests"})
			return
		}
	}

	// Validate
	if !validate.Email(request.Email) || !validate.Password(request.Password) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "invalid credentials"})
		return
	}

	// Get the user & check the password
	user, err := db.GetUserByEmail(request.Email)
	if err != nil || !user.Active || !user.CheckPassword(request.Password) {
		if cfg.AuthIPRateLimiting {
			until, _ := authratelimit.RecordFailure(clientIP, request.Email)
			gossipAuthFailure(&authratelimit.Event{
				IP: clientIP, Email: request.Email, At: time.Now(), BlockUntil: until,
			})
		}
		code := http.StatusUnauthorized

		audit.LogWithRequest(r,
			request.Email,
			model.AuditActorTypeUser,
			model.AuditEventAuthFailed,
			"",
			&map[string]interface{}{},
		)

		rest.WriteResponse(code, w, r, ErrorResponse{Error: "invalid email, password or TOTP code"})

		return
	}

	saveFields := []string{"LastLoginAt", "UpdatedAt"}

	// If TOTP is enabled
	if cfg.TOTP.Enabled {
		// If the user has a TOTP secret then check the code
		if user.TOTPSecret != "" {
			if !totp.VerifyCode(user.TOTPSecret, request.TOTPCode, cfg.TOTP.Window) {
				rest.WriteResponse(http.StatusUnauthorized, w, r, ErrorResponse{Error: "invalid email, password or TOTP code"})
				return
			}
		} else {
			// Generate a new TOTP secret
			user.TOTPSecret = totp.GenerateSecret()
			showTOTPSecret = user.TOTPSecret

			saveFields = append(saveFields, "TOTPSecret")
		}
	}

	// Update the last login time
	now := time.Now().UTC()
	user.LastLoginAt = &now
	user.UpdatedAt = hlc.Now()
	err = db.SaveUser(user, saveFields)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipUser(user)

	userId = user.Id

	// Create a session
	var session *model.Session = model.NewSession(r, userId)
	err = database.GetSessionStorage().SaveSession(session)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	service.GetTransport().GossipSession(session)
	sse.PublishSessionsChanged("")

	// Only create the cookie for web auth
	if r.URL.Path == "/api/auth/web" {
		cfg := config.GetServerConfig()
		// Drop any stale cookies first (e.g. a host-only cookie from before
		// wildcard-domain widening) so they can't shadow the fresh session.
		middleware.DeleteSessionCookie(w)
		cookie := &http.Cookie{
			Name:     model.WebSessionCookie,
			Value:    session.Id,
			Path:     "/",
			Domain:   cfg.SessionCookieDomain(),
			HttpOnly: true,
			Secure:   cfg.TLS.UseTLS,
			SameSite: http.SameSiteLaxMode,
		}

		http.SetCookie(w, cookie)
	}

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventAuthOk,
		"",
		&map[string]interface{}{
			"email": user.Email,
		},
	)

	// Remove rate limiters on successful login
	authratelimit.Clear(clientIP, request.Email)
	gossipAuthFailure(&authratelimit.Event{IP: clientIP, Email: request.Email, At: time.Now(), Clear: true})

	// Return the authentication token
	rest.WriteResponse(http.StatusOK, w, r, apiclient.AuthLoginResponse{
		Status:     true,
		Token:      session.Id,
		TOTPSecret: showTOTPSecret,
	})
}

func HandleLogout(w http.ResponseWriter, r *http.Request) {
	result := false
	value := r.Context().Value("session")

	if value != nil {
		session := value.(*model.Session)

		// Delete the session
		if session != nil {
			db := database.GetSessionStorage()
			session.IsDeleted = true
			session.ExpiresAfter = time.Now().Add(model.SessionExpiryDuration).UTC()
			session.UpdatedAt = hlc.Now()
			err := db.SaveSession(session)
			if err != nil {
				rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}
			service.GetTransport().GossipSession(session)

			result = true
		}
	}

	// Return the authentication token
	rest.WriteResponse(http.StatusOK, w, r, apiclient.AuthLogoutResponse{
		Status: result,
	})
}

// Returns if the server is using TOTP or not, the CLI client uses this to work out
// the authentication flow it should use.
func HandleUsingTotp(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetServerConfig()
	rest.WriteResponse(http.StatusOK, w, r, apiclient.UsingTOTPResponse{
		UsingTOTP: cfg.TOTP.Enabled,
	})
}

// gossipAuthFailure shares rate-limit state with the rest of the cluster so
// tracking and blocking are cluster-wide. A tripped block carries its
// deadline; a plain failure just counts everywhere; a clear resets.
func gossipAuthFailure(evt *authratelimit.Event) {
	if transport := service.GetTransport(); transport != nil {
		transport.GossipAuthFailure(evt)
	}
}
