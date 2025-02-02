package apiv1

import (
	"net/http"
	"time"

	"github.com/paularlott/knot/api/api_utils"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/internal/totp"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util/audit"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func HandleAuthorization(w http.ResponseWriter, r *http.Request) {
	var userId string = ""
	var tokenId string = ""
	var statusCode int = 0
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

	// If this is a remote then the request needs to be forwarded to the core server
	if server_info.IsLeaf {
		log.Debug().Msg("Forwarding auth request to origin server")

		client := apiclient.NewRemoteToken(viper.GetString("server.shared_token"))
		tokenId, showTOTPSecret, statusCode, err = client.Login(request.Email, request.Password, request.TOTPCode)
		if err != nil {
			if statusCode == http.StatusNotFound {
				// Look for the user by email and if found delete it
				user, err := db.GetUserByEmail(request.Email)
				if err == nil {
					api_utils.DeleteUser(db, user)
				}
			} else if statusCode == http.StatusLocked {
				// Look for the user by email and if found update it
				user, err := db.GetUserByEmail(request.Email)
				if err == nil && user.Active {
					user.Active = false
					db.SaveUser(user)
					api_utils.UpdateUserSpaces(user)
				}
			}

			rest.SendJSON(http.StatusUnauthorized, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Query the core server for the user details
		client.SetAuthToken(tokenId).UseSessionCookie(true)
		user, err := client.WhoAmI()
		if err != nil {
			rest.SendJSON(http.StatusUnauthorized, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// if restricted node then check token is in the users list
		if server_info.RestrictedLeaf {
			tokens, _, err := client.GetTokens()
			if err != nil {
				rest.SendJSON(http.StatusUnauthorized, w, r, ErrorResponse{Error: err.Error()})
				return
			}

			// check if one of the tokens matches server.shared_token
			found := false
			for _, token := range *tokens {
				if token.Id == viper.GetString("server.shared_token") {
					found = true
					break
				}
			}

			// if not found then return unauthorized
			if !found {
				rest.SendJSON(http.StatusUnauthorized, w, r, ErrorResponse{Error: "user restricted by leaf token"})
			}
		}

		// Store the user in the local database
		db := database.GetInstance()
		err = db.SaveUser(user)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		go api_utils.UpdateUserSpaces(user)

		userId = user.Id

	} else {

		// Get the user & check the password
		user, err := db.GetUserByEmail(request.Email)
		if err != nil || !user.Active || !user.CheckPassword(request.Password) {
			code := http.StatusUnauthorized

			// If request came from remote server then given more information
			if viper.GetString("server.shared_token") != "" && viper.GetString("server.shared_token") == middleware.GetBearerToken(w, r) {
				if user == nil {
					code = http.StatusNotFound
				} else if !user.Active {
					code = http.StatusLocked
				}
			}

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
			}
		}

		// Update the last login time
		now := time.Now().UTC()
		user.LastLoginAt = &now
		err = db.SaveUser(user)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		userId = user.Id
	}

	// Create a session
	var session *model.Session = model.NewSession(r, userId, tokenId)
	err = database.GetCacheInstance().SaveSession(session)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Only create the cookie for web auth
	if r.URL.Path == "/api/v1/auth/web" {
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

	if !server_info.IsLeaf {
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
	}

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
			err := database.GetCacheInstance().DeleteSession(session)
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
