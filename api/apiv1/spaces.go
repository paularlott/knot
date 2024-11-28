package apiv1

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/api/api_utils"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/origin_leaf/leaf"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/util/nomad"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func HandleGetSpaces(w http.ResponseWriter, r *http.Request) {
	var spaceData *apiclient.SpaceInfoList

	userId := r.URL.Query().Get("user_id")

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		var code int
		var err error

		spaceData, code, err = client.GetSpaces(userId)
		if err != nil {
			log.Error().Msgf("HandleGetSpaces: GetSpaces: %s", err.Error())
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		user := r.Context().Value("user").(*model.User)

		spaceData = &apiclient.SpaceInfoList{
			Count:  0,
			Spaces: []apiclient.SpaceInfo{},
		}

		var spaces []*model.Space
		var err error

		// If user doesn't have permission to manage spaces and filter user ID doesn't match the user return an empty list
		if !user.HasPermission(model.PermissionManageSpaces) && userId != user.Id {
			rest.SendJSON(http.StatusOK, w, r, spaceData)
			return
		}

		if userId == "" {
			spaces, err = db.GetSpaces()
			if err != nil {
				log.Error().Msgf("HandleGetSpaces: GetSpaces: %s", err.Error())
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}
		} else {
			spaces, err = db.GetSpacesForUser(userId)
			if err != nil {
				log.Error().Msgf("HandleGetSpaces: GetSpacesForUser: %s", err.Error())
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}
		}

		// Build a json array of space data to return to the client
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
			}

			s := apiclient.SpaceInfo{}

			s.Id = space.Id
			s.Name = space.Name
			s.TemplateName = templateName
			s.TemplateId = space.TemplateId
			s.Location = space.Location

			// Get the user
			u, err := db.GetUser(space.UserId)
			if err != nil {
				log.Error().Msgf("HandleGetSpaces: GetUser: %s", err.Error())
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}
			s.Username = u.Username
			s.UserId = u.Id

			s.VolumeSize, err = calcSpaceDiskUsage(space)
			if err != nil {
				log.Error().Msgf("HandleGetSpaces: calcSpaceDiskUsage: %s", err.Error())
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}

			spaceData.Spaces = append(spaceData.Spaces, s)
			spaceData.Count++
		}
	}

	rest.SendJSON(http.StatusOK, w, r, spaceData)
}

func HandleDeleteSpace(w http.ResponseWriter, r *http.Request) {
	var user *model.User = nil

	spaceId := chi.URLParam(r, "space_id")
	db := database.GetInstance()

	if r.Context().Value("user") != nil {
		user = r.Context().Value("user").(*model.User)
	}

	// Load the space if not found or doesn't belong to the user then treat both as not found
	space, err := db.GetSpace(spaceId)
	if err != nil || (user != nil && space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces)) {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: fmt.Sprintf("space %s not found", spaceId)})
		return
	}

	// If the space is running or changing state then fail
	if space.IsDeployed || space.IsPending || space.IsDeleting {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "space cannot be deleted"})
		return
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.DeleteSpace(spaceId)
		if err != nil {
			log.Error().Msgf("HandleDeleteSpace: %s", err.Error())
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// If space is running on this server then delete it from nomad
	if space.Location == server_info.LeafLocation {
		// Mark the space as deleting and delete it in the background
		space.IsDeleting = true
		db.SaveSpace(space)

		// Delete the space in the background
		RealDeleteSpace(space)
	} else {
		// Delete the space
		err = db.DeleteSpace(space)
		if err != nil {
			log.Error().Msgf("HandleDeleteSpace: %s", err.Error())
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		leaf.DeleteSpace(spaceId)
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
		log.Error().Msgf("HandleCreateSpace: %s", err.Error())
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Remove any blank alt names, any that match the primary name, and any duplicates
	request.AltNames = removeBlankAndDuplicates(request.AltNames, request.Name)

	// If user give and not our ID and no permission to manage spaces then fail
	if request.UserId != "" && request.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot create space for another user"})
		return
	}

	if !validate.Name(request.Name) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid name or template given for new space"})
		return
	}

	for _, altName := range request.AltNames {
		if !validate.Name(altName) {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid alt name given for space"})
			return
		}
	}

	if !validate.OneOf(request.Shell, []string{"bash", "zsh", "fish", "sh"}) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid shell given for space"})
		return
	}

	// Create the space
	forUserId := user.Id
	if request.UserId != "" {
		forUserId = request.UserId
	}
	space := model.NewSpace(request.Name, forUserId, request.TemplateId, request.Shell, &request.VolumeSizes, &request.AltNames)

	// Lock the space to the location of the server creating it
	if request.Location == "" {
		space.Location = server_info.LeafLocation
	} else {
		space.Location = request.Location
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.CreateSpace(space)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {

		// Test if over quota
		if user.MaxDiskSpace > 0 {
			// Lookup the template
			template, err := db.GetTemplate(request.TemplateId)
			if err != nil {
				rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Unknown template"})
				return
			}

			// Check the user and template have overlapping groups
			if len(template.Groups) > 0 && !user.HasAnyGroup(&template.Groups) {
				rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Unknown template"})
				return
			}

			// Get the size for this space
			size, err := space.GetStorageSize(template)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}

			// Get the size of storage for all the users spaces
			spaces, err := db.GetSpacesForUser(forUserId)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}

			for _, s := range spaces {
				sSize, err := calcSpaceDiskUsage(s)
				if err != nil {
					rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
					return
				}
				size += sSize
			}

			if size > user.MaxDiskSpace {
				rest.SendJSON(http.StatusInsufficientStorage, w, r, ErrorResponse{Error: "storage quota reached"})
				return
			}
		}
	}

	// Save the space
	err = database.GetInstance().SaveSpace(space)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	leaf.UpdateSpace(space)

	// Return the Token ID
	rest.SendJSON(http.StatusCreated, w, r, struct {
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
					log.Error().Msgf("HandleGetSpaceServiceState: %s", err.Error())
					rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
					return
				}

				// Save the space
				err = db.SaveSpace(space)
				if err != nil {
					log.Error().Msgf("HandleGetSpaceServiceState: %s", err.Error())
					rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
					return
				}
			} else {
				rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
				return
			}
		} else {
			log.Error().Msgf("HandleGetSpaceServiceState: %s", err.Error())
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	response := apiclient.SpaceServiceState{}
	state := agent_server.GetSession(spaceId)
	if state == nil {
		response.HasCodeServer = false
		response.HasSSH = false
		response.HasTerminal = false
		response.HasHttpVNC = false
		response.TcpPorts = make(map[string]string)
		response.HttpPorts = make(map[string]string)
		response.HasVSCodeTunnel = false
		response.VSCodeTunnel = ""
	} else {
		response.HasCodeServer = state.HasCodeServer
		response.HasSSH = state.SSHPort > 0
		response.HasTerminal = state.HasTerminal
		response.HasHttpVNC = state.VNCHttpPort > 0
		response.TcpPorts = state.TcpPorts

		// If wildcard domain is set then offer the http ports
		if viper.GetString("server.wildcard_domain") == "" {
			response.HttpPorts = make(map[string]string)
		} else {
			response.HttpPorts = state.HttpPorts
		}

		response.HasVSCodeTunnel = state.HasVSCodeTunnel
		response.VSCodeTunnel = state.VSCodeTunnelName
	}

	response.Name = space.Name
	response.Location = space.Location
	response.IsDeployed = space.IsDeployed
	response.IsPending = space.IsPending
	response.IsDeleting = space.IsDeleting
	response.IsRemote = space.Location != "" && space.Location != server_info.LeafLocation

	// If template is manual then force IsDeployed to true
	if space.TemplateId == model.MANUAL_TEMPLATE_ID {
		response.IsDeployed = true
	}

	// Check if the template has been updated
	hash := api_utils.GetTemplateHash(space.TemplateId)
	if space.TemplateId == model.MANUAL_TEMPLATE_ID || hash == "" {
		response.UpdateAvailable = false
	} else {
		response.UpdateAvailable = space.IsDeployed && space.TemplateHash != hash
	}

	rest.SendJSON(http.StatusOK, w, r, response)
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
		log.Error().Msgf("HandleSpaceStart: %s", err.Error())
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If user doesn't have permission to manage spaces and not their space then fail
	if user.Id != space.UserId && !user.HasPermission(model.PermissionManageSpaces) {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	// If the space is owned by a different user then load the user
	if user.Id != space.UserId {
		user, err = db.GetUser(space.UserId)
		if err != nil {
			log.Error().Msgf("HandleSpaceStart: %s", err.Error())
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// If remote then need to pull the space from the remote
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client = remoteClient.(*apiclient.ApiClient)

		var spaceRemote *model.Space
		spaceRemote, code, err = client.GetSpace(spaceId)
		if err != nil || space == nil {
			log.Error().Msgf("HandleSpaceStart: %s", err.Error())
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Load the space from disk and merge the remote space into it
		space.Name = spaceRemote.Name
		space.AltNames = spaceRemote.AltNames
		space.Shell = spaceRemote.Shell
		space.VolumeSizes = spaceRemote.VolumeSizes
	}

	// If the space is already running or changing state then fail
	if space.IsDeployed || space.IsPending || space.IsDeleting {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "space cannot be started"})
		return
	}

	// Is the space has a location then it must match the server location
	if space.Location != "" && space.Location != server_info.LeafLocation {
		rest.SendJSON(http.StatusNotAcceptable, w, r, ErrorResponse{Error: "space location does not match server location"})
		return
	}

	// Mark the space as pending and save it
	space.IsPending = true
	if err = db.SaveSpace(space); err != nil {
		log.Error().Msgf("HandleSpaceStart: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Update the origin server
	if client != nil {
		code, err = client.UpdateSpace(space)
		if err != nil {
			log.Error().Msgf("HandleSpaceStart: %s", err.Error())
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// Revert the pending status if the deploy fails
	var deployFailed = true
	defer func() {
		if deployFailed {
			// If the deploy failed then revert the space to not pending
			space.IsPending = false
			db.SaveSpace(space)

			// If remote then need to update the remote
			if client != nil {
				code, err = client.UpdateSpace(space)
				if err != nil {
					log.Error().Msgf("HandleSpaceStart: %s", err.Error())
				}
			}
		}
	}()

	// Get the template
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		log.Error().Msgf("HandleSpaceStart: get template %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Get the variables
	variables, err := db.GetTemplateVars()
	if err != nil {
		log.Error().Msgf("HandleSpaceStart: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	vars := model.FilterVars(variables)

	// Get the nomad client
	nomadClient := nomad.NewClient()

	// Create volumes
	err = nomadClient.CreateSpaceVolumes(user, template, space, &vars)
	if err != nil {
		log.Error().Msgf("HandleSpaceStart: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Start the job
	err = nomadClient.CreateSpaceJob(user, template, space, &vars)
	if err != nil {
		log.Error().Msgf("HandleSpaceStart: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)

	// Don't revert the space on success
	deployFailed = false
}

func HandleSpaceStop(w http.ResponseWriter, r *http.Request) {
	var err error
	var code int
	var space *model.Space
	var client *apiclient.ApiClient = nil

	user := r.Context().Value("user").(*model.User)
	spaceId := chi.URLParam(r, "space_id")
	db := database.GetInstance()

	space, err = db.GetSpace(spaceId)
	if err != nil {
		log.Error().Msgf("HandleSpaceStop: %s", err.Error())
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If user doesn't have permission to manage spaces and not their space then fail
	if user.Id != space.UserId && !user.HasPermission(model.PermissionManageSpaces) {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	// If the space is not running or changing state then fail
	if (!space.IsDeployed && !space.IsPending) || space.IsDeleting {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "space cannot be stopped"})
		return
	}

	// If remote then need to pull the space from the remote
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client = remoteClient.(*apiclient.ApiClient)

		var spaceRemote *model.Space
		spaceRemote, code, err = client.GetSpace(spaceId)
		if err != nil || space == nil {
			log.Error().Msgf("HandleSpaceStop: %s", err.Error())
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Load the space from disk and merge the remote space into it
		space.Name = spaceRemote.Name
		space.AltNames = spaceRemote.AltNames
		space.Shell = spaceRemote.Shell
		space.VolumeSizes = spaceRemote.VolumeSizes
	}

	// Mark the space as pending and save it
	space.IsPending = true
	if err = db.SaveSpace(space); err != nil {
		log.Error().Msgf("HandleSpaceStart: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Update the origin server
	if client != nil {
		code, err = client.UpdateSpace(space)
		if err != nil {
			log.Error().Msgf("HandleSpaceStart: %s", err.Error())
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// Get the nomad client
	nomadClient := nomad.NewClient()

	// Stop the job
	err = nomadClient.DeleteSpaceJob(space)
	if err != nil {
		space.IsPending = false
		db.SaveSpace(space)
		if client != nil {
			client.UpdateSpace(space)
		}

		log.Error().Msgf("HandleSpaceStop: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
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
		log.Error().Msgf("HandleUpdateSpace: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if user != nil && space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	request := apiclient.UpdateSpaceRequest{}
	err = rest.BindJSON(w, r, &request)
	if err != nil {
		log.Error().Msgf("HandleUpdateSpace: %s", err.Error())
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Remove any blank alt names, any that match the primary name, and any duplicates
	request.AltNames = removeBlankAndDuplicates(request.AltNames, request.Name)

	if !validate.Name(request.Name) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid name or template given for new space"})
		return
	}

	for _, altName := range request.AltNames {
		if !validate.Name(altName) {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid alt name given for space"})
			return
		}
	}

	if !validate.OneOf(request.Shell, []string{"bash", "zsh", "fish", "sh"}) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid shell given for space"})
		return
	}

	// Update the space
	space.Name = request.Name
	space.TemplateId = request.TemplateId
	space.Shell = request.Shell
	space.AltNames = request.AltNames

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.UpdateSpace(space)
		if err != nil {
			log.Error().Msgf("HandleUpdateSpace: %s", err.Error())
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		// Lookup the template
		_, err = db.GetTemplate(request.TemplateId)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Unknown template"})
			return
		}
	}

	err = db.SaveSpace(space)
	if err != nil {
		log.Error().Msgf("HandleUpdateSpace: %s", err.Error())
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If the space is in a pending state then don't notify the leaf servers as another update will be coming, avoids a race condition
	if !space.IsPending {
		leaf.UpdateSpace(space)
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
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	for _, space := range spaces {
		if space.IsDeployed && (space.Location == "" || space.Location == server_info.LeafLocation) {
			err = nomadClient.DeleteSpaceJob(space)
			if err != nil {
				log.Error().Msgf("HandleSpaceStopUsersSpaces: %s", err.Error())
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			} else {
				// If remote then need to update status on remote
				remoteClient := r.Context().Value("remote_client")
				if remoteClient != nil {
					client := remoteClient.(*apiclient.ApiClient)
					code, err := client.UpdateSpace(space)
					if err != nil {
						log.Error().Msgf("HandleSpaceStopUsersSpaces: %s", err.Error())
						rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
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
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		var spaceRemote *model.Space
		spaceRemote, code, err = client.GetSpace(spaceId)
		if err != nil || space == nil {
			log.Error().Msgf("HandleGetSpace: %s", err.Error())
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Merge the remote space into it
		space.Name = spaceRemote.Name
		space.AltNames = spaceRemote.AltNames
		space.Shell = spaceRemote.Shell
		space.VolumeSizes = spaceRemote.VolumeSizes

		// Save the space
		err = database.GetInstance().SaveSpace(space)
		if err != nil {
			log.Error().Msgf("HandleGetSpace: %s", err.Error())
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		if r.Context().Value("user") != nil {
			user := r.Context().Value("user").(*model.User)
			if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
				rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
				return
			}
		}
	}

	response := apiclient.SpaceDefinition{
		UserId:      space.UserId,
		TemplateId:  space.TemplateId,
		Name:        space.Name,
		Shell:       space.Shell,
		Location:    space.Location,
		AltNames:    space.AltNames,
		IsDeployed:  space.IsDeployed,
		IsPending:   space.IsPending,
		IsDeleting:  space.IsDeleting,
		VolumeSizes: space.VolumeSizes,
		VolumeData:  space.VolumeData,
	}

	rest.SendJSON(http.StatusOK, w, r, &response)
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

func RealDeleteSpace(space *model.Space) {
	go func() {
		log.Info().Msgf("api: RealDeleteSpace: deleting %s", space.Id)

		db := database.GetInstance()

		// Get the nomad client
		nomadClient := nomad.NewClient()

		// Delete volumes on failure we log the error and revert the space to not deleting
		err := nomadClient.DeleteSpaceVolumes(space)
		if err != nil {
			log.Error().Msgf("api: RealDeleteSpace %s", err.Error())

			space.IsDeleting = false
			db.SaveSpace(space)
			return
		}

		// Delete the agent state if present
		agent_server.RemoveSession(space.Id)

		// Delete the space
		err = db.DeleteSpace(space)
		if err != nil {
			log.Error().Msgf("api: RealDeleteSpace %s", err.Error())

			space.IsDeleting = false
			db.SaveSpace(space)
			return
		}

		log.Info().Msgf("api: RealDeleteSpace: deleted %s", space.Id)
	}()

	leaf.DeleteSpace(space.Id)
}
