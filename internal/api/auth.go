package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/totp"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
	"github.com/rs/zerolog/log"

	"github.com/spf13/viper"
	"golang.org/x/time/rate"
)

// Add metadata to track last use
type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastUsed time.Time
}

var (
	// Rate limit by IP address
	ipLimiters = make(map[string]*rateLimiterEntry)
	ipMutex    sync.Mutex

	// Rate limit by email address
	emailLimiters = make(map[string]*rateLimiterEntry)
	emailMutex    sync.Mutex
)

const (
	cleanupInterval = 10 * time.Minute
	cleanupMaxAge   = 30 * time.Minute

	ipRateLimit  = 5 // requests per minute
	ipBurstLimit = 3 // burst size

	emailRateLimit  = 3 // requests per minute
	emailBurstLimit = 3 // burst size
)

func cleanupLimiters(ctx context.Context) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cleanupTime := time.Now().Add(-cleanupMaxAge)

			// Cleanup IP limiters
			ipMutex.Lock()
			for ip, entry := range ipLimiters {
				if entry.lastUsed.Before(cleanupTime) {
					delete(ipLimiters, ip)
				}
			}
			ipMutex.Unlock()

			// Cleanup email limiters
			emailMutex.Lock()
			for email, entry := range emailLimiters {
				if entry.lastUsed.Before(cleanupTime) {
					delete(emailLimiters, email)
				}
			}
			emailMutex.Unlock()

		case <-ctx.Done():
			return
		}
	}
}

// Helper function to get or create a rate limiter
func getIPLimiter(ip string) *rate.Limiter {
	ipMutex.Lock()
	defer ipMutex.Unlock()

	entry, exists := ipLimiters[ip]
	if !exists {
		// Allow ipRateLimit requests per minute with burst of ipBurstLimit
		limiter := rate.NewLimiter(rate.Limit(ipRateLimit)/60.0, ipBurstLimit)
		entry = &rateLimiterEntry{
			limiter:  limiter,
			lastUsed: time.Now(),
		}
		ipLimiters[ip] = entry
	} else {
		entry.lastUsed = time.Now()
	}

	return entry.limiter
}

// Similar function for email
func getEmailLimiter(email string) *rate.Limiter {
	emailMutex.Lock()
	defer emailMutex.Unlock()

	entry, exists := emailLimiters[email]
	if !exists {
		// Allow emailRateLimit requests per minute with burst of emailBurstLimit
		limiter := rate.NewLimiter(rate.Limit(emailRateLimit)/60.0, emailBurstLimit)
		entry = &rateLimiterEntry{
			limiter:  limiter,
			lastUsed: time.Now(),
		}
		emailLimiters[email] = entry
	} else {
		entry.lastUsed = time.Now()
	}

	return entry.limiter
}

// Remove limiters on successful login
func removeRateLimiters(email, ip string) {
	// Remove IP limiter
	ipMutex.Lock()
	delete(ipLimiters, ip)
	ipMutex.Unlock()

	// Remove email limiter
	emailMutex.Lock()
	delete(emailLimiters, email)
	emailMutex.Unlock()
}

func HandleAuthorization(w http.ResponseWriter, r *http.Request) {
	var userId string = ""
	var showTOTPSecret string = ""

	db := database.GetInstance()
	request := apiclient.AuthLoginRequest{}

	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Get client IP (consistent with how we got it for rate limiting)
	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}

	if viper.GetBool("server.auth_ip_rate_limiting") {
		// Apply rate limiting by IP
		ipLimiter := getIPLimiter(clientIP)
		if !ipLimiter.Allow() {
			log.Warn().Msgf("Rate limit exceeded for IP: %s", clientIP)
			rest.SendJSON(http.StatusTooManyRequests, w, r, ErrorResponse{Error: "too many requests"})
			return
		}
	}

	// Apply rate limiting by email
	emailLimiter := getEmailLimiter(request.Email)
	if !emailLimiter.Allow() {
		log.Warn().Msgf("Rate limit exceeded for email: %s", request.Email)
		rest.SendJSON(http.StatusTooManyRequests, w, r, ErrorResponse{Error: "too many requests"})
		return
	}

	// Validate
	if !validate.Email(request.Email) || !validate.Password(request.Password) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "invalid credentials"})
		return
	}

	// Get the user & check the password
	user, err := db.GetUserByEmail(request.Email)
	if err != nil || !user.Active || !user.CheckPassword(request.Password) {
		code := http.StatusUnauthorized

		audit.Log(
			request.Email,
			model.AuditActorTypeUser,
			model.AuditEventAuthFailed,
			"",
			&map[string]interface{}{
				"agent": r.UserAgent(),
				"IP":    clientIP,
			},
		)

		rest.SendJSON(code, w, r, ErrorResponse{Error: "invalid email, password or TOTP code"})

		return
	}

	saveFields := []string{"LastLoginAt", "UpdatedAt"}

	// If TOTP is enabled
	if viper.GetBool("server.totp.enabled") {
		// If the user has a TOTP secret then check the code
		if user.TOTPSecret != "" {
			if !totp.VerifyCode(user.TOTPSecret, request.TOTPCode, viper.GetInt("server.totp.window")) {
				rest.SendJSON(http.StatusUnauthorized, w, r, ErrorResponse{Error: "invalid email, password or TOTP code"})
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
	user.UpdatedAt = now
	err = db.SaveUser(user, saveFields)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipUser(user)

	userId = user.Id

	// Create a session
	var session *model.Session = model.NewSession(r, userId)
	err = database.GetSessionStorage().SaveSession(session)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	service.GetTransport().GossipSession(session)

	// Only create the cookie for web auth
	if r.URL.Path == "/api/auth/web" {
		cookie := &http.Cookie{
			Name:     model.WebSessionCookie,
			Value:    session.Id,
			Path:     "/",
			HttpOnly: true,
			Secure:   viper.GetBool("server.tls.use_tls"),
			SameSite: http.SameSiteLaxMode,
		}

		http.SetCookie(w, cookie)
	}

	audit.Log(
		request.Email,
		model.AuditActorTypeUser,
		model.AuditEventAuthOk,
		"",
		&map[string]interface{}{
			"agent": r.UserAgent(),
			"IP":    clientIP,
		},
	)

	// Remove rate limiters on successful login
	removeRateLimiters(request.Email, clientIP)

	// Return the authentication token
	rest.SendJSON(http.StatusOK, w, r, apiclient.AuthLoginResponse{
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
			session.UpdatedAt = time.Now().UTC()
			err := db.SaveSession(session)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}
			service.GetTransport().GossipSession(session)

			result = true
		}
	}

	// Return the authentication token
	rest.SendJSON(http.StatusOK, w, r, apiclient.AuthLogoutResponse{
		Status: result,
	})
}

// Returns if the server is using TOTP or not, the CLI client uses this to work out
// the authentication flow it should use.
func HandleUsingTotp(w http.ResponseWriter, r *http.Request) {
	rest.SendJSON(http.StatusOK, w, r, apiclient.UsingTOTPResponse{
		UsingTOTP: viper.GetBool("server.totp.enabled"),
	})
}
