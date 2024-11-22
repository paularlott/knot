package apiv1

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/api/api_utils"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/leaf"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

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
		userData, err := client.GetUsers(requiredState, server_info.LeafLocation)
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
					if space.IsDeployed || space.IsPending {
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
	var existsLocal bool = true
	var user *model.User
	var client *apiclient.ApiClient = nil
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client = remoteClient.(*apiclient.ApiClient)

		user, err = client.GetUser(userId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
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
			rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
			return
		}
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

	// If on leaf
	if client != nil {
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

	if existsLocal {
		// Save
		err = db.SaveUser(user)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Update the user's spaces, ssh keys or stop spaces
		go api_utils.UpdateUserSpaces(user)

		// notify all remote servers of the change
		leaf.UpdateUser(user)
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
		if err := api_utils.DeleteUser(db, toDelete); err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// If core server then notify all remote servers of the change
	leaf.DeleteUser(userId)

	w.WriteHeader(http.StatusOK)
}
