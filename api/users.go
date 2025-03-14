package api

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/api/api_utils"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/leaf"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/internal/tunnel_server"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/audit"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
)

func HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	newUserId := ""

	db := database.GetInstance()
	request := apiclient.CreateUserRequest{}

	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Validate
	if !validate.Name(request.Username) ||
		!validate.Password(request.Password) ||
		!validate.Email(request.Email) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid username, password, or email given for new user"})
		return
	}
	if !validate.OneOf(request.PreferredShell, []string{"bash", "zsh", "fish", "sh"}) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid shell"})
		return
	}
	if !validate.MaxLength(request.SSHPublicKey, 16*1024) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "SSH public key too long"})
		return
	}
	if !validate.OneOf(request.Timezone, util.Timezones) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid timezone"})
		return
	}
	if !validate.IsNumber(int(request.MaxSpaces), 0, 10000) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid max spaces"})
		return
	}
	if !validate.IsPositiveNumber(int(request.ComputeUnits)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid compute units"})
		return
	}
	if !validate.IsPositiveNumber(int(request.StorageUnits)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid storage units"})
		return
	}
	if !validate.IsPositiveNumber(int(request.MaxTunnels)) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid tunnel limit"})
		return
	}
	if !validate.MaxLength(request.GitHubUsername, 255) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "GitHub username too long"})
		return
	}

	// Check roles give are present in the system, if not drop them
	userRoles := []string{}
	for _, role := range request.Roles {
		if model.RoleExists(role) {
			userRoles = append(userRoles, role)
		}
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code := 0
		newUserId, code, err = client.CreateUser(&request)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		// Check the groups are present in the system
		for _, groupId := range request.Groups {
			_, err := db.GetGroup(groupId)
			if err != nil {
				rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: fmt.Sprintf("Group %s does not exist", groupId)})
				return
			}
		}

		// Create the user
		userNew := model.NewUser(request.Username, request.Email, request.Password, userRoles, request.Groups, request.SSHPublicKey, request.PreferredShell, request.Timezone, request.MaxSpaces, request.GitHubUsername, request.ComputeUnits, request.StorageUnits, request.MaxTunnels)
		if request.ServicePassword != "" {
			userNew.ServicePassword = request.ServicePassword
		}
		err = db.SaveUser(userNew, nil)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		newUserId = userNew.Id

		// Don't log the initial setup
		if middleware.HasUsers {
			user := r.Context().Value("user").(*model.User)
			audit.Log(
				user.Username,
				model.AuditActorTypeUser,
				model.AuditEventUserCreate,
				fmt.Sprintf("Created user %s (%s)", userNew.Username, userNew.Email),
				&map[string]interface{}{
					"agent":           r.UserAgent(),
					"IP":              r.RemoteAddr,
					"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
					"user_id":         userNew.Id,
					"user_name":       userNew.Username,
					"user_email":      userNew.Email,
				},
			)
		}

		// Tell the middleware that users are present
		middleware.HasUsers = true
	}

	// Return the user ID
	rest.SendJSON(http.StatusCreated, w, r, &apiclient.CreateUserResponse{
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

	userId := r.PathValue("user_id")

	if !validate.UUID(userId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		user, err = client.GetUser(userId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		user, err = db.GetUser(userId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
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
		ComputeUnits:    user.ComputeUnits,
		StorageUnits:    user.StorageUnits,
		MaxTunnels:      user.MaxTunnels,
		SSHPublicKey:    user.SSHPublicKey,
		GitHubUsername:  user.GitHubUsername,
		PreferredShell:  user.PreferredShell,
		TOTPSecret:      user.TOTPSecret,
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

	rest.SendJSON(http.StatusOK, w, r, &userData)
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
		ComputeUnits:    user.ComputeUnits,
		StorageUnits:    user.StorageUnits,
		MaxTunnels:      user.MaxTunnels,
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

	rest.SendJSON(http.StatusOK, w, r, &userData)
}

func HandleGetUsers(w http.ResponseWriter, r *http.Request) {
	activeUser := r.Context().Value("user").(*model.User)
	requiredState := r.URL.Query().Get("state")
	inLocation := r.URL.Query().Get("location")
	local := r.URL.Query().Get("local") == "true"

	if requiredState == "" {
		requiredState = "all"
	}

	// If no location given then use the servers
	if inLocation == "" {
		inLocation = server_info.LeafLocation
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil && !local {
		client := remoteClient.(*apiclient.ApiClient)
		userData, err := client.GetUsers(requiredState, server_info.LeafLocation)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, userData)
	} else {
		var userData = &apiclient.UserInfoList{
			Count: 0,
			Users: []apiclient.UserInfo{},
		}

		db := database.GetInstance()
		users, err := db.GetUsers()
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
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
				data.ComputeUnits = user.ComputeUnits
				data.StorageUnits = user.StorageUnits
				data.MaxTunnels = user.MaxTunnels
				data.Current = user.Id == activeUser.Id

				// Get the users quota
				quota, err := database.GetUserQuota(user)
				if err != nil {
					rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
					return
				}

				data.MaxSpaces = quota.MaxSpaces
				data.ComputeUnits = quota.ComputeUnits
				data.StorageUnits = quota.StorageUnits
				data.MaxTunnels = quota.MaxTunnels

				if user.LastLoginAt != nil {
					t := user.LastLoginAt.UTC()
					data.LastLoginAt = &t
				} else {
					data.LastLoginAt = nil
				}

				// Get the users usage
				usage, err := database.GetUserUsage(user.Id, inLocation)
				if err != nil {
					rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
					return
				}

				data.NumberSpaces = usage.NumberSpaces
				data.NumberSpacesDeployed = usage.NumberSpacesDeployed
				data.NumberSpacesDeployedInLocation = usage.NumberSpacesDeployedInLocation
				data.UsedComputeUnits = usage.ComputeUnits
				data.UsedStorageUnits = usage.StorageUnits

				userData.Users = append(userData.Users, data)
				userData.Count++
			}
		}

		rest.SendJSON(http.StatusOK, w, r, userData)
	}
}

func HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	activeUser := r.Context().Value("user").(*model.User)
	userId := r.PathValue("user_id")

	if !validate.UUID(userId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	request := apiclient.UpdateUserRequest{}
	db := database.GetInstance()

	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Validate
	if len(request.Password) > 0 && !validate.Password(request.Password) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid password given"})
		return
	}
	if !validate.Email(request.Email) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid email"})
		return
	}
	if !validate.OneOf(request.PreferredShell, []string{"bash", "zsh", "fish", "sh"}) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid shell"})
		return
	}
	if !validate.MaxLength(request.SSHPublicKey, 16*1024) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "SSH public key too long"})
		return
	}
	if !validate.OneOf(request.Timezone, util.Timezones) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid timezone"})
		return
	}
	if !validate.MaxLength(request.GitHubUsername, 255) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "GitHub username too long"})
		return
	}

	// Load the existing user
	var existsLocal bool = true
	var user *model.User
	var client *apiclient.ApiClient = nil
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client = remoteClient.(*apiclient.ApiClient)

		user, err = client.GetUser(userId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Test if the user exists locally
		_, err = db.GetUser(userId)
		if err != nil {
			existsLocal = false
		}
	} else {
		user, err = db.GetUser(userId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	user.Email = request.Email
	user.SSHPublicKey = request.SSHPublicKey
	user.GitHubUsername = request.GitHubUsername
	user.PreferredShell = request.PreferredShell
	user.Timezone = request.Timezone
	user.TOTPSecret = request.TOTPSecret

	if request.ServicePassword != "" {
		user.ServicePassword = request.ServicePassword
	}

	if activeUser.HasPermission(model.PermissionManageUsers) {
		if !validate.IsNumber(int(request.MaxSpaces), 0, 10000) {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid max spaces"})
			return
		}

		if !validate.IsPositiveNumber(int(request.ComputeUnits)) {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid compute units"})
			return
		}

		if !validate.IsPositiveNumber(int(request.StorageUnits)) {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid storage units"})
			return
		}
		if !validate.IsPositiveNumber(int(request.MaxTunnels)) {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid tunnel limit"})
			return
		}

		// Check roles give are present in the system, if not drop them
		userRoles := []string{}
		for _, role := range request.Roles {
			if model.RoleExists(role) {
				userRoles = append(userRoles, role)
			}
		}

		if activeUser.Id != user.Id {
			user.Active = request.Active
		}

		user.Roles = userRoles
		user.Groups = request.Groups
		user.MaxSpaces = request.MaxSpaces
		user.ComputeUnits = request.ComputeUnits
		user.StorageUnits = request.StorageUnits
		user.MaxTunnels = request.MaxTunnels
	}

	saveFields := []string{"Email", "SSHPublicKey", "GitHubUsername", "PreferredSheel", "Timezone", "TOTPSecret", "Active", "Roles", "Groups", "MaxSpaces", "ComputeUnits", "StorageUnits", "MaxTunnels"}

	// If on leaf
	if client != nil {
		user.Password = request.Password

		err = client.UpdateUser(user)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		// Update the user password
		if len(request.Password) > 0 {
			user.SetPassword(request.Password)
			saveFields = append(saveFields, "Password")
		}

		// Check the groups are present in the system
		for _, groupId := range request.Groups {
			_, err := db.GetGroup(groupId)
			if err != nil {
				rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: fmt.Sprintf("Group %s does not exist", groupId)})
				return
			}
		}

		audit.Log(
			activeUser.Username,
			model.AuditActorTypeUser,
			model.AuditEventUserUpdate,
			fmt.Sprintf("Updated user %s (%s)", user.Username, user.Email),
			&map[string]interface{}{
				"agent":           r.UserAgent(),
				"IP":              r.RemoteAddr,
				"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
				"user_id":         user.Id,
				"user_name":       user.Username,
				"user_email":      user.Email,
			},
		)
	}

	if existsLocal {
		// Save
		err = db.SaveUser(user, saveFields)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Update the user's spaces, ssh keys or stop spaces
		go api_utils.UpdateUserSpaces(user)

		// notify all remote servers of the change
		leaf.UpdateUser(user, saveFields, nil)
	}

	w.WriteHeader(http.StatusOK)
}

func HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)
	userId := r.PathValue("user_id")

	if !validate.UUID(userId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	// If trying to delete self then fail
	if user.Id == userId {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Cannot delete self"})
		return
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		err := client.DeleteUser(userId)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// Load the user to delete
	toDelete, err := db.GetUser(userId)
	if err != nil && remoteClient == nil {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: fmt.Sprintf("user %s not found", userId)})
		return
	}

	if toDelete != nil {
		if err := api_utils.DeleteUser(db, toDelete); err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	if remoteClient == nil && toDelete != nil {
		audit.Log(
			user.Username,
			model.AuditActorTypeUser,
			model.AuditEventUserDelete,
			fmt.Sprintf("Deleted user %s (%s)", toDelete.Username, toDelete.Email),
			&map[string]interface{}{
				"agent":           r.UserAgent(),
				"IP":              r.RemoteAddr,
				"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
				"user_id":         toDelete.Id,
				"user_name":       toDelete.Username,
				"user_email":      toDelete.Email,
			},
		)
	}

	// If core server then notify all remote servers of the change
	leaf.DeleteUser(userId, nil)

	w.WriteHeader(http.StatusOK)
}

func HandleGetUserQuota(w http.ResponseWriter, r *http.Request) {
	db := database.GetInstance()
	userId := r.PathValue("user_id")

	if !validate.UUID(userId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)
		userQuota, err := client.GetUserQuota(userId)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, userQuota)
		return
	}

	// Load the user, if not found return 404
	user, err := db.GetUser(userId)
	if err != nil || user == nil {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "user not found"})
		return
	}

	usage, err := database.GetUserUsage(userId, "")
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	userQuota, err := database.GetUserQuota(user)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	quota := apiclient.UserQuota{
		MaxSpaces:    userQuota.MaxSpaces,
		ComputeUnits: userQuota.ComputeUnits,
		StorageUnits: userQuota.StorageUnits,
		MaxTunnels:   userQuota.MaxTunnels,

		NumberSpaces:         usage.NumberSpaces,
		NumberSpacesDeployed: usage.NumberSpacesDeployed,
		UsedComputeUnits:     usage.ComputeUnits,
		UsedStorageUnits:     usage.StorageUnits,
		UsedTunnels:          tunnel_server.CountUserTunnels(userId),
	}

	rest.SendJSON(http.StatusOK, w, r, quota)
}
