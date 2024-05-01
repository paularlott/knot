package apiv1

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/api/agentv1"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/nomad"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"github.com/go-chi/chi/v5"
)

func HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	newUserId := ""

	db := database.GetInstance()
	request := apiclient.CreateUserRequest{}

	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Validate
	if !validate.Name(request.Username) ||
		!validate.Password(request.Password) ||
		!validate.Email(request.Email) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid username, password, or email given for new user"})
		return
	}
	if !validate.OneOf(request.PreferredShell, []string{"bash", "zsh", "fish", "sh"}) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid shell"})
		return
	}
	if !validate.MaxLength(request.SSHPublicKey, 16*1024) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "SSH public key too long"})
		return
	}
	if !validate.OneOf(request.Timezone, util.Timezones) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid timezone"})
		return
	}
	if !validate.IsNumber(int(request.MaxSpaces), 0, 1000) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid max spaces"})
		return
	}
	if !validate.IsNumber(int(request.MaxDiskSpace), 0, 1000000) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid max disk space"})
		return
	}
	if !validate.MaxLength(request.GitHubUsername, 255) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "GitHub username too long"})
		return
	}

	// Check roles give are present in the system
	for _, role := range request.Roles {
		if !model.RoleExists(role) {
			rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Role %s does not exist", role)})
			return
		}
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code := 0
		newUserId, code, err = client.CreateUser(&request)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		// Check the groups are present in the system
		for _, groupId := range request.Groups {
			_, err := db.GetGroup(groupId)
			if err != nil {
				rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Group %s does not exist", groupId)})
				return
			}
		}

		// Create the user
		userNew := model.NewUser(request.Username, request.Email, request.Password, request.Roles, request.Groups, request.SSHPublicKey, request.PreferredShell, request.Timezone, request.MaxSpaces, request.MaxDiskSpace, request.GitHubUsername)
		if request.ServicePassword != "" {
			userNew.ServicePassword = request.ServicePassword
		}
		err = db.SaveUser(userNew)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
			return
		}

		newUserId = userNew.Id

		// Tell the middleware that users are present
		middleware.HasUsers = true
	}

	// Return the user ID
	rest.SendJSON(http.StatusCreated, w, &apiclient.CreateUserResponse{
		Status: true,
		UserId: newUserId,
	})
}

func HandleGetUser(w http.ResponseWriter, r *http.Request) {
	var user *model.User
	var activeUser *model.User = nil
	var err error

	db := database.GetInstance()

	if r.Context().Value("user") != nil {
		activeUser = r.Context().Value("user").(*model.User)
	}

	userId := chi.URLParam(r, "user_id")

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		user, err = client.GetUser(userId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Save the user local
		err = db.SaveUser(user)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		user, err = db.GetUser(userId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// Build a json array of data to return to the client
	userData := apiclient.UserResponse{
		Id:              user.Id,
		Username:        user.Username,
		Email:           user.Email,
		ServicePassword: user.ServicePassword,
		Roles:           user.Roles,
		Groups:          user.Groups,
		Active:          user.Active,
		MaxSpaces:       user.MaxSpaces,
		MaxDiskSpace:    user.MaxDiskSpace,
		SSHPublicKey:    user.SSHPublicKey,
		GitHubUsername:  user.GitHubUsername,
		PreferredShell:  user.PreferredShell,
		Timezone:        user.Timezone,
		Current:         activeUser != nil && user.Id == activeUser.Id,
		LastLoginAt:     nil,
		CreatedAt:       user.CreatedAt.UTC(),
		UpdatedAt:       user.UpdatedAt.UTC(),
	}

	if user.LastLoginAt != nil {
		t := user.LastLoginAt.UTC()
		userData.LastLoginAt = &t
	}

	rest.SendJSON(http.StatusOK, w, &userData)
}

func HandleWhoAmI(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	// Build a json array of data to return to the client
	userData := apiclient.UserResponse{
		Id:              user.Id,
		Username:        user.Username,
		Email:           user.Email,
		ServicePassword: user.ServicePassword,
		Roles:           user.Roles,
		Groups:          user.Groups,
		Active:          user.Active,
		MaxSpaces:       user.MaxSpaces,
		MaxDiskSpace:    user.MaxDiskSpace,
		SSHPublicKey:    user.SSHPublicKey,
		GitHubUsername:  user.GitHubUsername,
		PreferredShell:  user.PreferredShell,
		Timezone:        user.Timezone,
		Current:         true,
		LastLoginAt:     nil,
		CreatedAt:       user.CreatedAt.UTC(),
		UpdatedAt:       user.UpdatedAt.UTC(),
	}

	if user.LastLoginAt != nil {
		t := user.LastLoginAt.UTC()
		userData.LastLoginAt = &t
	}

	rest.SendJSON(http.StatusOK, w, &userData)
}

func HandleGetUsers(w http.ResponseWriter, r *http.Request) {
	activeUser := r.Context().Value("user").(*model.User)
	requiredState := r.URL.Query().Get("state")
	inLocation := r.URL.Query().Get("location")
	if requiredState == "" {
		requiredState = "all"
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		userData, err := client.GetUsers(requiredState, viper.GetString("server.location"))
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, userData)
	} else {
		var userData = &apiclient.UserInfoList{
			Count: 0,
			Users: []apiclient.UserInfo{},
		}

		db := database.GetInstance()
		users, err := db.GetUsers()
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		for _, user := range users {
			if requiredState == "all" || (requiredState == "active" && user.Active) || (requiredState == "inactive" && !user.Active) {
				data := apiclient.UserInfo{}

				data.Id = user.Id
				data.Username = user.Username
				data.Email = user.Email
				data.Roles = user.Roles
				data.Groups = user.Groups
				data.Active = user.Active
				data.MaxSpaces = user.MaxSpaces
				data.MaxDiskSpace = user.MaxDiskSpace
				data.Current = user.Id == activeUser.Id

				if user.LastLoginAt != nil {
					t := user.LastLoginAt.UTC()
					data.LastLoginAt = &t
				} else {
					data.LastLoginAt = nil
				}

				// Find the number of spaces the user has
				spaces, err := db.GetSpacesForUser(user.Id)
				if err != nil {
					rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
					return
				}

				var deployed int = 0
				var deployedInLocation int = 0
				var diskSpace int = 0
				for _, space := range spaces {
					if space.IsDeployed {
						deployed++

						if inLocation != "" && space.Location == inLocation {
							deployedInLocation++
						}
					}

					sSize, err := calcSpaceDiskUsage(space)
					if err != nil {
						rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
						return
					}

					diskSpace += sSize
				}

				data.NumberSpaces = len(spaces)
				data.NumberSpacesDeployed = deployed
				data.NumberSpacesDeployedInLocation = deployedInLocation
				data.UsedDiskSpace = diskSpace

				userData.Users = append(userData.Users, data)
				userData.Count++
			}
		}

		rest.SendJSON(http.StatusOK, w, userData)
	}
}

func HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	activeUser := r.Context().Value("user").(*model.User)
	userId := chi.URLParam(r, "user_id")
	request := apiclient.UpdateUserRequest{}
	db := database.GetInstance()

	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Validate
	if len(request.Password) > 0 && !validate.Password(request.Password) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid password given"})
		return
	}
	if !validate.Email(request.Email) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid email"})
		return
	}
	if !validate.OneOf(request.PreferredShell, []string{"bash", "zsh", "fish", "sh"}) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid shell"})
		return
	}
	if !validate.MaxLength(request.SSHPublicKey, 16*1024) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "SSH public key too long"})
		return
	}
	if !validate.OneOf(request.Timezone, util.Timezones) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid timezone"})
		return
	}
	if !validate.MaxLength(request.GitHubUsername, 255) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "GitHub username too long"})
		return
	}

	// Load the existing user
	user, err := db.GetUser(userId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
		return
	}

	user.Email = request.Email
	user.SSHPublicKey = request.SSHPublicKey
	user.GitHubUsername = request.GitHubUsername
	user.PreferredShell = request.PreferredShell
	user.Timezone = request.Timezone

	if request.ServicePassword != "" {
		user.ServicePassword = request.ServicePassword
	}

	if activeUser.HasPermission(model.PermissionManageUsers) {
		if !validate.IsNumber(int(request.MaxSpaces), 0, 1000) {
			rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid max spaces"})
			return
		}

		// Check roles give are present in the system
		for _, role := range request.Roles {
			if !model.RoleExists(role) {
				rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Role %s does not exist", role)})
				return
			}
		}

		if activeUser.Id != user.Id {
			user.Active = request.Active
		}

		user.Roles = request.Roles
		user.Groups = request.Groups
		user.MaxSpaces = request.MaxSpaces
		user.MaxDiskSpace = request.MaxDiskSpace
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		user.Password = request.Password

		err = client.UpdateUser(user)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		// Update the user password
		if len(request.Password) > 0 {
			user.SetPassword(request.Password)
		}

		// Check the groups are present in the system
		for _, groupId := range request.Groups {
			_, err := db.GetGroup(groupId)
			if err != nil {
				rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Group %s does not exist", groupId)})
				return
			}
		}
	}

	// Save
	err = db.SaveUser(user)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Update the user's spaces, ssh keys or stop spaces
	go UpdateUserSpaces(user)

	// If core server then notify all remote servers of the change
	if viper.GetBool("server.is_core") {
		go func() {
			remoteServers, err := database.GetCacheInstance().GetRemoteServers()
			if err != nil {
				log.Error().Msgf("Failed to get remote servers: %s", err)
			} else {
				for _, remoteServer := range remoteServers {
					log.Debug().Msgf("Notifying remote server %s of update of user %s", remoteServer.Url, user.Username)

					client := apiclient.NewRemoteServerClient(remoteServer.Url)
					err := client.NotifyRemoteUserUpdate(user.Id)
					if err != nil {
						log.Error().Msgf("Failed to notify remote server %s of update for user %s: %s", remoteServer.Url, user.Username, err)
					}
				}
			}
		}()
	}

	w.WriteHeader(http.StatusOK)
}

func HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)
	userId := chi.URLParam(r, "user_id")

	// If trying to delete self then fail
	if user.Id == userId {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Cannot delete self"})
		return
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		err := client.DeleteUser(userId)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// Load the user to delete
	toDelete, err := db.GetUser(userId)
	if err != nil && remoteClient == nil {
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: fmt.Sprintf("user %s not found", userId)})
		return
	}

	if toDelete != nil {
		if err := DeleteUser(db, toDelete); err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// If core server then notify all remote servers of the change
	if viper.GetBool("server.is_core") {
		go func() {
			remoteServers, err := database.GetCacheInstance().GetRemoteServers()
			if err != nil {
				log.Error().Msgf("Failed to get remote servers: %s", err)
			} else {
				for _, remoteServer := range remoteServers {
					log.Debug().Msgf("Notifying remote server %s of delete of user %s", remoteServer.Url, toDelete.Username)

					client := apiclient.NewRemoteServerClient(remoteServer.Url)
					err := client.NotifyRemoteUserDelete(toDelete.Id)
					if err != nil {
						log.Error().Msgf("Failed to notify remote server %s of delete of user %s: %s", remoteServer.Url, toDelete.Username, err)
					}
				}
			}
		}()
	}

	w.WriteHeader(http.StatusOK)
}

func DeleteUser(db database.IDbDriver, toDelete *model.User) error {
	var hasError = false

	log.Debug().Msgf("delete user: Deleting user %s", toDelete.Id)

	// Stop all spaces and delete all volumes
	spaces, err := db.GetSpacesForUser(toDelete.Id)
	if err != nil {
		return err
	}

	// If this is a remote then tell the core server of the space update
	var api *apiclient.ApiClient = nil
	if viper.GetBool("server.is_remote") {
		api = apiclient.NewRemoteToken(viper.GetString("server.remote_token"))
	}

	// Get the nomad client
	nomadClient := nomad.NewClient()
	for _, space := range spaces {
		log.Debug().Msgf("delete user: Deleting space %s", space.Id)

		if space.Location == viper.GetString("server.location") {
			log.Debug().Msgf("delete user: Deleting space %s from nomad", space.Id)

			// Stop the job
			if space.IsDeployed {
				err = nomadClient.DeleteSpaceJob(space)
				if err != nil {
					log.Debug().Msgf("delete user: Failed to delete space job %s: %s", space.Id, err)
					hasError = true
					break
				}
			}

			// Delete the volumes
			err = nomadClient.DeleteSpaceVolumes(space)
			if err != nil {
				log.Debug().Msgf("delete user: Failed to delete space volumes %s: %s", space.Id, err)
				hasError = true
				break
			}

			// Notify the core server
			if api != nil {
				_, err := api.RemoteDeleteSpace(space.Id)
				if err != nil {
					log.Error().Msgf("Failed to delete space %s on core server: %s", space.Id, err)
				}
			}
		}

		db.DeleteSpace(space)
	}

	// Delete the user
	if !hasError {
		err = db.DeleteUser(toDelete)
		if err != nil {
			return err
		}

		removeUsersSessions(toDelete)
		removeUsersTokens(toDelete)
	}

	return nil
}

// Delete the sessions owned by a user
func removeUsersSessions(user *model.User) {
	cache := database.GetCacheInstance()

	// Find sessions for the user and delete them
	sessions, err := cache.GetSessionsForUser(user.Id)
	if err == nil && sessions != nil {
		for _, session := range sessions {
			cache.DeleteSession(session)
		}
	}
}

// Delete the tokens owned by a user
func removeUsersTokens(user *model.User) {
	db := database.GetInstance()

	// Find API tokens for the user and delete them
	tokens, err := db.GetTokensForUser(user.Id)
	if err == nil && tokens != nil {
		for _, token := range tokens {
			db.DeleteToken(token)
		}
	}
}

func updateSpacesSSHKey(user *model.User) {
	db := database.GetInstance()
	cache := database.GetCacheInstance()

	log.Debug().Msgf("Updating agent SSH key for user %s", user.Id)

	// Load the list of spaces for the user
	spaces, err := db.GetSpacesForUser(user.Id)
	if err != nil {
		log.Debug().Msgf("Failed to get spaces for user %s: %s", user.Id, err)
		return
	}

	// Loop through all spaces updating the active ones
	for _, space := range spaces {
		if space.IsDeployed {
			// Get the agent state
			agentState, err := cache.GetAgentState(space.Id)
			if err != nil || agentState == nil {
				// Silently ignore if space is on a different server
				if space.Location == "" || space.Location == viper.GetString("server.location") {
					log.Debug().Msgf("Agent state not found for space %s", space.Id)
				}
				continue
			}

			// If agent accepting SSH keys then update
			if agentState.SSHPort > 0 {
				log.Debug().Msgf("Sending SSH public key to agent %s", space.Id)
				client := rest.NewClient(util.ResolveSRVHttp(space.GetAgentURL()), agentState.AccessToken, viper.GetBool("tls_skip_verify"))
				if !agentv1.CallAgentUpdateAuthorizedKeys(client, user.SSHPublicKey, user.GitHubUsername) {
					log.Debug().Msg("Failed to send SSH public key to agent")
				}
			}
		}
	}

	log.Debug().Msgf("Finished updating agent SSH key for user %s", user.Id)
}

// For disabled users ensure all spaces are stopped, for enabled users update the SSH key on the agents
func UpdateUserSpaces(user *model.User) {
	// If the user is disabled then stop all spaces
	if !user.Active {
		spaces, err := database.GetInstance().GetSpacesForUser(user.Id)
		if err != nil {
			return
		}

		// If this is a remote then tell the core server of the space update
		var api *apiclient.ApiClient = nil
		if viper.GetBool("server.is_remote") {
			api = apiclient.NewRemoteToken(viper.GetString("server.remote_token"))
		}

		// Get the nomad client
		nomadClient := nomad.NewClient()
		for _, space := range spaces {
			if space.IsDeployed && (space.Location == "" || space.Location == viper.GetString("server.location")) {
				nomadClient.DeleteSpaceJob(space)

				if api != nil {
					_, err := api.RemoteUpdateSpace(space)
					if err != nil {
						log.Error().Msgf("Failed to update space %s: %s", space.Id, err)
					}
				}
			}
		}

		// Kill the sessions to logout the user, but leave the tokens there until they expire
		removeUsersSessions(user)
	} else {
		// Update the SSH key on the agents
		updateSpacesSSHKey(user)
	}
}
