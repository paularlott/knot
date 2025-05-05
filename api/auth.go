package api

import (
	"net/http"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/cluster"
	"github.com/paularlott/knot/internal/totp"
	"github.com/paularlott/knot/util/audit"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/spf13/viper"
)

func HandleAuthorization(w http.ResponseWriter, r *http.Request) {
	var userId string = ""
	var tokenId string = ""
	var showTOTPSecret string = ""

	db := database.GetInstance()
	request := apiclient.AuthLoginRequest{}

	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Validate
	if !validate.Email(request.Email) || !validate.Password(request.Password) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "invalid email or password"})
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
				"agent":           r.UserAgent(),
				"IP":              r.RemoteAddr,
				"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
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

	cluster.GetInstance().GossipUser(user)

	userId = user.Id

	// Create a session
	var session *model.Session = model.NewSession(r, userId, tokenId)
	err = database.GetSessionStorage().SaveSession(session)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Only create the cookie for web auth
	if r.URL.Path == "/api/auth/web" {
		cookie := &http.Cookie{
			Name:     model.WEBUI_SESSION_COOKIE,
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
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
		},
	)

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
			err := database.GetSessionStorage().DeleteSession(session)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}

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
