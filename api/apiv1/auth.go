package apiv1

import (
	"net/http"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func HandleAuthorization(w http.ResponseWriter, r *http.Request) {
	var userId string = ""
	var tokenId string = ""
	var statusCode int = 0
	db := database.GetInstance()
	request := apiclient.AuthLoginRequest{}

	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Validate
	if !validate.Email(request.Email) || !validate.Password(request.Password) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "invalid email or password"})
		return
	}

	// If this is a remote then the request needs to be forwarded to the core server
	if viper.GetBool("server.is_remote") {
		log.Debug().Msg("Forwarding request to core server")

		client := apiclient.NewRemoteSession("")
		tokenId, statusCode, err = client.Login(request.Email, request.Password)
		if err != nil {
			if statusCode == http.StatusNotFound {
				// Look for the user by email and if found delete it
				user, err := db.GetUserByEmail(request.Email)
				if err == nil {
					DeleteUser(db, user)
				}
			} else if statusCode == http.StatusLocked {
				// Look for the user by email and if found update it
				user, err := db.GetUserByEmail(request.Email)
				if err == nil && user.Active {
					user.Active = false
					db.SaveUser(user)
					UpdateUserSpaces(user)
				}
			}

			rest.SendJSON(http.StatusUnauthorized, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Query the core server for the user details
		client.SetAuthToken(tokenId)
		user, err := client.WhoAmI()
		if err != nil {
			rest.SendJSON(http.StatusUnauthorized, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Store the user in the local database
		db := database.GetInstance()
		err = db.SaveUser(user)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		go UpdateUserSpaces(user)

		userId = user.Id

	} else {

		// Get the user & check the password
		user, err := db.GetUserByEmail(request.Email)
		if err != nil || !user.Active || !user.CheckPassword(request.Password) {
			code := http.StatusUnauthorized

			// If request came from remote server then given more information
			if viper.GetString("server.remote_token") != "" {
				if viper.GetString("remote_token") == middleware.GetBearerToken(w, r) {
					if user == nil {
						code = http.StatusNotFound
					} else if !user.Active {
						code = http.StatusLocked
					}
				}
			}

			rest.SendJSON(code, w, ErrorResponse{Error: "invalid email or password"})

			return
		}

		// Update the last login time
		now := time.Now().UTC()
		user.LastLoginAt = &now
		err = db.SaveUser(user)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		userId = user.Id
	}

	// Create a session
	var session *model.Session = model.NewSession(r, userId, tokenId)
	err = database.GetCacheInstance().SaveSession(session)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Only create the cookie for web auth
	if r.URL.Path == "/api/v1/auth/web" {
		cookie := &http.Cookie{
			Name:     model.WEBUI_SESSION_COOKIE,
			Value:    session.Id,
			Path:     "/",
			HttpOnly: true,
			Secure:   false,
			SameSite: http.SameSiteLaxMode,
		}

		http.SetCookie(w, cookie)
	}

	// Return the authentication token
	rest.SendJSON(http.StatusOK, w, apiclient.AuthLoginResponse{
		Status: true,
		Token:  session.Id,
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
				rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
				return
			}

			result = true
		}
	}

	// Return the authentication token
	rest.SendJSON(http.StatusOK, w, apiclient.AuthLogoutResponse{
		Status: result,
	})
}
