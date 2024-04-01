package apiv1

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/nomad"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/viper"
)

func HandleGetSpaces(w http.ResponseWriter, r *http.Request) {
	userId := r.URL.Query().Get("user_id")
	spaceData := []*apiclient.SpaceInfo{}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		var code int
		var err error

		spaceData, code, err = client.GetSpaces(userId)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		user := r.Context().Value("user").(*model.User)

		// If user doesn't have permission to manage spaces and filter user ID doesn't match the user return an empty list
		if !user.HasPermission(model.PermissionManageSpaces) && userId != user.Id {
			rest.SendJSON(http.StatusOK, w, []struct{}{})
			return
		}

		var spaces []*model.Space
		var err error

		if userId == "" {
			spaces, err = db.GetSpaces()
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
				return
			}
		} else {
			spaces, err = db.GetSpacesForUser(userId)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
				return
			}
		}

		// Build a json array of token data to return to the client
		for _, space := range spaces {
			var templateName string

			if space.TemplateId != model.MANUAL_TEMPLATE_ID {
				// Lookup the template
				template, err := db.GetTemplate(space.TemplateId)
				if err != nil {
					templateName = "Unknown"
				} else {
					templateName = template.Name
				}
			} else {
				templateName = "None (" + space.AgentURL + ")"
			}

			s := &apiclient.SpaceInfo{}

			s.Id = space.Id
			s.Name = space.Name
			s.TemplateName = templateName
			s.TemplateId = space.TemplateId
			s.Location = space.Location

			// Get the user
			u, err := db.GetUser(space.UserId)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
				return
			}
			s.Username = u.Username
			s.UserId = u.Id

			s.VolumeSize, err = calcSpaceDiskUsage(space)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
				return
			}

			spaceData = append(spaceData, s)
		}
	}

	rest.SendJSON(http.StatusOK, w, spaceData)
}

func HandleDeleteSpace(w http.ResponseWriter, r *http.Request) {
	var user *model.User = nil

	spaceId := chi.URLParam(r, "space_id")
	db := database.GetInstance()
	cache := database.GetCacheInstance()

	if r.Context().Value("user") != nil {
		user = r.Context().Value("user").(*model.User)
	}

	// Load the space if not found or doesn't belong to the user then treat both as not found
	space, err := db.GetSpace(spaceId)
	if err != nil || (user != nil && space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces)) {
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: fmt.Sprintf("space %s not found", spaceId)})
		return
	}

	// If the space is running then fail
	if space.IsDeployed {
		rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "space is running"})
		return
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.DeleteSpace(spaceId)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// If space is running on this server then delete it from nomad
	if space.Location == viper.GetString("server.location") {
		// Get the nomad client
		nomadClient := nomad.NewClient()

		// Delete volumes
		err = nomadClient.DeleteSpaceVolumes(space)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Delete the agent state if present
		state, _ := cache.GetAgentState(space.Id)
		if state != nil {
			cache.DeleteAgentState(state)
		}
	}

	// Delete the space
	err = db.DeleteSpace(space)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func removeBlankAndDuplicates(names []string, primary string) []string {
	encountered := map[string]bool{}
	var newNames []string
	for _, name := range names {
		if name != "" && name != primary && !encountered[name] {
			encountered[name] = true
			newNames = append(newNames, name)
		}
	}
	return newNames
}

func HandleCreateSpace(w http.ResponseWriter, r *http.Request) {
	db := database.GetInstance()
	request := apiclient.CreateSpaceRequest{}
	user := r.Context().Value("user").(*model.User)

	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Remove any blank alt names, any that match the primary name, and any duplicates
	request.AltNames = removeBlankAndDuplicates(request.AltNames, request.Name)

	// If user give and not our ID and no permission to manage spaces then fail
	if request.UserId != "" && request.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		rest.SendJSON(http.StatusForbidden, w, ErrorResponse{Error: "Cannot create space for another user"})
		return
	}

	// If template given then ensure the address is removed
	if request.TemplateId != model.MANUAL_TEMPLATE_ID {
		request.AgentURL = ""
	}

	if !validate.Name(request.Name) || (request.TemplateId == model.MANUAL_TEMPLATE_ID && !validate.Uri(request.AgentURL)) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid name, template, or address given for new space"})
		return
	}

	for _, altName := range request.AltNames {
		if !validate.Name(altName) {
			rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid alt name given for space"})
			return
		}
	}

	if !validate.OneOf(request.Shell, []string{"bash", "zsh", "fish", "sh"}) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid shell given for space"})
		return
	}

	// Create the space
	forUserId := user.Id
	if request.UserId != "" {
		forUserId = request.UserId
	}
	space := model.NewSpace(request.Name, forUserId, request.AgentURL, request.TemplateId, request.Shell, &request.VolumeSizes, &request.AltNames)

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.CreateSpace(space)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {

		// Test if over quota
		if user.MaxDiskSpace > 0 {
			// Lookup the template
			template, err := db.GetTemplate(request.TemplateId)
			if err != nil {
				rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Unknown template"})
				return
			}

			// Check the user and template have overlapping groups
			if len(template.Groups) > 0 && !user.HasAnyGroup(&template.Groups) {
				rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Unknown template"})
				return
			}

			// Get the size for this space
			size, err := space.GetStorageSize(template)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
				return
			}

			// Get the size of storage for all the users spaces
			spaces, err := db.GetSpacesForUser(forUserId)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
				return
			}

			for _, s := range spaces {
				sSize, err := calcSpaceDiskUsage(s)
				if err != nil {
					rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
					return
				}
				size += sSize
			}

			if size > user.MaxDiskSpace {
				rest.SendJSON(http.StatusInsufficientStorage, w, ErrorResponse{Error: "storage quota reached"})
				return
			}
		}
	}

	// Save the space
	err = database.GetInstance().SaveSpace(space)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Return the Token ID
	rest.SendJSON(http.StatusCreated, w, struct {
		Status  bool   `json:"status"`
		SpaceID string `json:"space_id"`
	}{
		Status:  true,
		SpaceID: space.Id,
	})
}

func HandleGetSpaceServiceState(w http.ResponseWriter, r *http.Request) {
	spaceId := chi.URLParam(r, "space_id")

	db := database.GetInstance()
	cache := database.GetCacheInstance()

	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil {
		if err.Error() == "space not found" {
			// Try to get space from remote
			remoteClient := r.Context().Value("remote_client")
			if remoteClient != nil {
				client := remoteClient.(*apiclient.ApiClient)

				var code int
				var err error

				space, code, err = client.GetSpace(spaceId)
				if err != nil || space == nil {
					rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
					return
				}

				// Save the space
				err = db.SaveSpace(space)
				if err != nil {
					rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
					return
				}
			} else {
				rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
				return
			}
		} else {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}
	}

	response := apiclient.SpaceServiceState{}
	state, _ := cache.GetAgentState(spaceId)
	if state == nil {
		response.HasCodeServer = false
		response.HasSSH = false
		response.HasTerminal = false
		response.HasHttpVNC = false
		response.TcpPorts = []int{}
		response.HttpPorts = []int{}
	} else {
		response.HasCodeServer = state.HasCodeServer
		response.HasSSH = state.SSHPort > 0
		response.HasTerminal = state.HasTerminal
		response.HasHttpVNC = state.VNCHttpPort > 0
		response.TcpPorts = state.TcpPorts

		// If wildcard domain is set then offer the http ports
		if viper.GetString("server.wildcard_domain") == "" {
			response.HttpPorts = []int{}
		} else {
			response.HttpPorts = state.HttpPorts
		}
	}

	response.Name = space.Name
	response.Location = space.Location
	response.IsDeployed = space.IsDeployed

	// TODO Implement the template update available check as a periodic job, add hash to get templates call, cache the value in ram
	/* 	if space.TemplateId == model.MANUAL_TEMPLATE_ID {
	   		response.UpdateAvailable = false
	   	} else {
	   		template, err := db.GetTemplate(space.TemplateId)
	   		if err != nil {
	   			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
	   			return
	   		}
	   		response.UpdateAvailable = space.IsDeployed && space.TemplateHash != template.Hash
	   	} */

	rest.SendJSON(http.StatusOK, w, response)
}

func HandleSpaceStart(w http.ResponseWriter, r *http.Request) {
	var err error
	var code int
	var space *model.Space
	var client *apiclient.ApiClient = nil

	spaceId := chi.URLParam(r, "space_id")
	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	space, err = db.GetSpace(spaceId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
		return
	}

	// If remote then need to pull the space from the remote
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client = remoteClient.(*apiclient.ApiClient)

		var spaceRemote *model.Space
		spaceRemote, code, err = client.GetSpace(spaceId)
		if err != nil || space == nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Load the space from disk and merge the remote space into it
		space.Name = spaceRemote.Name
		space.AltNames = spaceRemote.AltNames
		space.AgentURL = spaceRemote.AgentURL
		space.Shell = spaceRemote.Shell
		space.VolumeSizes = spaceRemote.VolumeSizes
	}

	// If the space is already running then fail
	if space.IsDeployed {
		rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "space is running"})
		return
	}

	// Is the space has a location then it must match the server location
	if space.Location != "" && space.Location != viper.GetString("server.location") {
		rest.SendJSON(http.StatusNotAcceptable, w, ErrorResponse{Error: "space location does not match server location"})
		return
	}

	var template *model.Template
	var variables []*model.TemplateVar

	if client != nil {
		// Open new client with remote access
		clientRemote := apiclient.NewRemoteToken(viper.GetString("server.remote_token"))

		// Get the template
		template, code, err = clientRemote.GetTemplateObject(space.TemplateId)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Get the template variables
		variables, code, err = clientRemote.GetTemplateVarValues()
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		// Get the template
		template, err = db.GetTemplate(space.TemplateId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Get the variables
		variables, err = db.GetTemplateVars()
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}
	}

	space.Location = viper.GetString("server.location")

	vars := make(map[string]interface{})
	for _, variable := range variables {
		vars[variable.Name] = variable.Value
	}

	// Get the nomad client
	nomadClient := nomad.NewClient()

	// Create volumes
	err = nomadClient.CreateSpaceVolumes(user, template, space, &vars)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Start the job
	err = nomadClient.CreateSpaceJob(user, template, space, &vars)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Update the remote
	if client != nil {
		code, err = client.UpdateSpace(space)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func HandleSpaceStop(w http.ResponseWriter, r *http.Request) {
	var err error
	var code int
	var space *model.Space
	var client *apiclient.ApiClient = nil

	spaceId := chi.URLParam(r, "space_id")
	db := database.GetInstance()
	cache := database.GetCacheInstance()

	space, err = db.GetSpace(spaceId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
		return
	}

	// If the space is not running then fail
	if !space.IsDeployed {
		rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "space not running"})
		return
	}

	// If remote then need to pull the space from the remote
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client = remoteClient.(*apiclient.ApiClient)

		var spaceRemote *model.Space
		spaceRemote, code, err = client.GetSpace(spaceId)
		if err != nil || space == nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Load the space from disk and merge the remote space into it
		space.Name = spaceRemote.Name
		space.AltNames = spaceRemote.AltNames
		space.AgentURL = spaceRemote.AgentURL
		space.Shell = spaceRemote.Shell
		space.VolumeSizes = spaceRemote.VolumeSizes
	}

	// Get the nomad client
	nomadClient := nomad.NewClient()

	// Stop the job
	err = nomadClient.DeleteSpaceJob(space)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Record the space as not deployed
	space.IsDeployed = false
	db.SaveSpace(space)

	// Delete the agent state
	state, _ := cache.GetAgentState(space.Id)
	if state != nil {
		cache.DeleteAgentState(state)
	}

	// Update the remote
	if client != nil {
		code, err = client.UpdateSpace(space)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func HandleUpdateSpace(w http.ResponseWriter, r *http.Request) {
	var user *model.User = nil
	spaceId := chi.URLParam(r, "space_id")

	db := database.GetInstance()

	if r.Context().Value("user") != nil {
		user = r.Context().Value("user").(*model.User)
	}

	space, err := db.GetSpace(spaceId)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	if user != nil && space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "space not found"})
		return
	}

	request := apiclient.UpdateSpaceRequest{}
	err = rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	// If template given then ensure the address is removed
	if request.TemplateId != model.MANUAL_TEMPLATE_ID {
		request.AgentURL = ""
	}

	// Remove any blank alt names, any that match the primary name, and any duplicates
	request.AltNames = removeBlankAndDuplicates(request.AltNames, request.Name)

	if !validate.Name(request.Name) || (request.TemplateId == model.MANUAL_TEMPLATE_ID && !validate.Uri(request.AgentURL)) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid name, template, or address given for new space"})
		return
	}

	for _, altName := range request.AltNames {
		if !validate.Name(altName) {
			rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid alt name given for space"})
			return
		}
	}

	if !validate.OneOf(request.Shell, []string{"bash", "zsh", "fish", "sh"}) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid shell given for space"})
		return
	}

	// Update the space
	space.Name = request.Name
	space.TemplateId = request.TemplateId
	space.AgentURL = request.AgentURL
	space.Shell = request.Shell
	space.AltNames = request.AltNames
	space.IsDeployed = request.IsDeployed
	space.Location = request.Location

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.UpdateSpace(space)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		// Lookup the template
		_, err = db.GetTemplate(request.TemplateId)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Unknown template"})
			return
		}
	}

	err = db.SaveSpace(space)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleSpaceStopUsersSpaces(w http.ResponseWriter, r *http.Request) {
	db := database.GetInstance()

	// Get the nomad client
	nomadClient := nomad.NewClient()

	// Stop all spaces
	spaces, err := db.GetSpacesForUser(chi.URLParam(r, "user_id"))
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
		return
	}

	for _, space := range spaces {
		if space.IsDeployed && (space.Location == "" || space.Location == viper.GetString("server.location")) {
			err = nomadClient.DeleteSpaceJob(space)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
				return
			} else {
				// If remote then need to update status on remote
				remoteClient := r.Context().Value("remote_client")
				if remoteClient != nil {
					client := remoteClient.(*apiclient.ApiClient)
					code, err := client.UpdateSpace(space)
					if err != nil {
						rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
						return
					}
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func HandleGetSpace(w http.ResponseWriter, r *http.Request) {
	var space *model.Space
	var err error
	var code int

	spaceId := chi.URLParam(r, "space_id")
	db := database.GetInstance()

	space, err = db.GetSpace(spaceId)
	if err != nil || space == nil {
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
		return
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		var spaceRemote *model.Space
		spaceRemote, code, err = client.GetSpace(spaceId)
		if err != nil || space == nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Merge the remote space into it
		space.Name = spaceRemote.Name
		space.AltNames = spaceRemote.AltNames
		space.AgentURL = spaceRemote.AgentURL
		space.Shell = spaceRemote.Shell
		space.VolumeSizes = spaceRemote.VolumeSizes

		// Save the space
		err = database.GetInstance().SaveSpace(space)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		user := r.Context().Value("user").(*model.User)
		if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
			rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "space not found"})
			return
		}
	}

	response := apiclient.SpaceDefinition{
		UserId:      space.UserId,
		TemplateId:  space.TemplateId,
		Name:        space.Name,
		AgentURL:    space.AgentURL,
		Shell:       space.Shell,
		Location:    space.Location,
		AltNames:    space.AltNames,
		IsDeployed:  space.IsDeployed,
		VolumeSizes: space.VolumeSizes,
		VolumeData:  space.VolumeData,
	}

	rest.SendJSON(http.StatusOK, w, &response)
}

func calcSpaceDiskUsage(space *model.Space) (int, error) {
	tmpl, err := database.GetInstance().GetTemplate(space.TemplateId)
	if err != nil {
		return 0, err
	}

	size, err := space.GetStorageSize(tmpl)
	if err != nil {
		return 0, err
	}

	return size, nil
}
