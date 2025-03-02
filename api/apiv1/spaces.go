package apiv1

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/knot/api/api_utils"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/container"
	"github.com/paularlott/knot/internal/container/docker"
	"github.com/paularlott/knot/internal/container/nomad"
	"github.com/paularlott/knot/internal/origin_leaf/leaf"
	"github.com/paularlott/knot/internal/origin_leaf/origin"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/util/audit"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

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
			var localContainer bool
			var isManual bool

			// Lookup the template
			template, err := db.GetTemplate(space.TemplateId)
			if err != nil {
				templateName = "Unknown"
				localContainer = false
				isManual = false
			} else {
				templateName = template.Name
				localContainer = template.LocalContainer
				isManual = template.IsManual
			}

			s := apiclient.SpaceInfo{}

			s.Id = space.Id
			s.Name = space.Name
			s.TemplateName = templateName
			s.TemplateId = space.TemplateId
			s.Location = space.Location
			s.LocalContainer = localContainer
			s.IsManual = isManual

			// Get the user
			u, err := db.GetUser(space.UserId)
			if err != nil {
				log.Error().Msgf("HandleGetSpaces: GetUser: %s", err.Error())
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}
			s.Username = u.Username
			s.UserId = u.Id

			// If shared with another user then lookup the user
			s.SharedUserId = ""
			s.SharedUsername = ""
			if space.SharedWithUserId != "" {
				u, err = db.GetUser(space.SharedWithUserId)
				if err == nil {
					s.SharedUserId = u.Id
					s.SharedUsername = u.Username
				}
			}

			spaceData.Spaces = append(spaceData.Spaces, s)
			spaceData.Count++
		}
	}

	rest.SendJSON(http.StatusOK, w, r, spaceData)
}

func HandleDeleteSpace(w http.ResponseWriter, r *http.Request) {
	var user *model.User = nil
	spaceId := r.PathValue("space_id")

	if !validate.UUID(spaceId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid space ID"})
		return
	}

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
	} else {
		audit.Log(
			user.Username,
			model.AuditActorTypeUser,
			model.AuditEventSpaceDelete,
			fmt.Sprintf("Deleted space %s", space.Name),
			&map[string]interface{}{
				"agent":           r.UserAgent(),
				"IP":              r.RemoteAddr,
				"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
				"space_id":        space.Id,
				"space_name":      space.Name,
			},
		)
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

	db := database.GetInstance()

	// Create the space
	if request.UserId != "" {
		user, err = db.GetUser(request.UserId)
		if err != nil {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}
	space := model.NewSpace(request.Name, user.Id, request.TemplateId, request.Shell, &request.AltNames)

	// Lock the space to the location of the server creating it
	if request.Location == "" {
		space.Location = server_info.LeafLocation
	} else {
		space.Location = request.Location
	}

	// If space create is disabled then fail
	if viper.GetBool("server.disable_space_create") && space.Location == server_info.LeafLocation {
		rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "Space creation is disabled"})
		return
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

		// Get the groups and build a map
		groups, err := db.GetGroups()
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		groupMap := make(map[string]*model.Group)
		for _, group := range groups {
			groupMap[group.Id] = group
		}

		maxSpaces := user.MaxSpaces
		for _, groupId := range user.Groups {
			group, ok := groupMap[groupId]
			if ok {
				maxSpaces += group.MaxSpaces
			}
		}

		// Get the number of spaces for the user
		if maxSpaces > 0 {
			spaces, err := db.GetSpacesForUser(user.Id)
			if err != nil {
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}

			if uint32(len(spaces)) >= maxSpaces {
				rest.SendJSON(http.StatusInsufficientStorage, w, r, ErrorResponse{Error: "space quota exceeded"})
				return
			}
		}
	}

	// Save the space
	err = db.SaveSpace(space)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if remoteClient == nil {
		audit.Log(
			user.Username,
			model.AuditActorTypeUser,
			model.AuditEventSpaceCreate,
			fmt.Sprintf("Created space %s", space.Name),
			&map[string]interface{}{
				"agent":           r.UserAgent(),
				"IP":              r.RemoteAddr,
				"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
				"space_id":        space.Id,
				"space_name":      space.Name,
			},
		)
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
	spaceId := r.PathValue("space_id")

	if !validate.UUID(spaceId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid space ID"})
		return
	}

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
					log.Error().Msgf("HandleGetSpaceServiceState: GetSpace from origin %s", err.Error())
					rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
					return
				}

				// Save the space
				err = db.SaveSpace(space)
				if err != nil {
					log.Error().Msgf("HandleGetSpaceServiceState: SaveSpace %s", err.Error())
					rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
					return
				}
			} else {
				rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
				return
			}
		} else {
			log.Error().Msgf("HandleGetSpaceServiceState: GetSpace %s", err.Error())
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		log.Error().Msgf("HandleGetSpaceServiceState: GetTemplate %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	response := apiclient.SpaceServiceState{}
	response.Name = space.Name
	response.Location = space.Location
	response.IsDeployed = space.IsDeployed
	response.IsPending = space.IsPending
	response.IsDeleting = space.IsDeleting
	response.IsRemote = space.Location != "" && space.Location != server_info.LeafLocation

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
		response.HasState = false
	} else {
		response.HasCodeServer = state.HasCodeServer
		response.HasSSH = state.SSHPort > 0
		response.HasTerminal = state.HasTerminal
		response.HasHttpVNC = state.VNCHttpPort > 0
		response.TcpPorts = state.TcpPorts
		response.HasState = true

		// If wildcard domain is set then offer the http ports
		if viper.GetString("server.wildcard_domain") == "" {
			response.HttpPorts = make(map[string]string)
		} else {
			response.HttpPorts = state.HttpPorts
		}

		response.HasVSCodeTunnel = state.HasVSCodeTunnel
		response.VSCodeTunnel = state.VSCodeTunnelName

		// If template is manual then force IsDeployed to true as agent is live
		if template.IsManual {
			response.IsDeployed = true
		}
	}

	// Check if the template has been updated
	hash := api_utils.GetTemplateHash(space.TemplateId)
	if template.IsManual || hash == "" {
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

	spaceId := r.PathValue("space_id")

	if !validate.UUID(spaceId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid space ID"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	space, err = db.GetSpace(spaceId)
	if err != nil {
		log.Error().Msgf("HandleSpaceStart: %s", err.Error())
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If user doesn't have permission to manage spaces and not their space then fail
	if user.Id != space.UserId && user.Id != space.SharedWithUserId && !user.HasPermission(model.PermissionManageSpaces) {
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

	var quota *apiclient.UserQuota

	// If remote then need to pull the space from the remote
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client = remoteClient.(*apiclient.ApiClient)

		quota, err = client.GetUserQuota(user.Id)
		if err != nil {
			log.Error().Msgf("HandleSpaceStart: %s", err.Error())
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

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
	} else {
		usage, err := database.GetUserUsage(user.Id, "")
		if err != nil {
			log.Error().Msgf("HandleSpaceStart: %s", err.Error())
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		quota = &apiclient.UserQuota{
			MaxSpaces:            user.MaxSpaces,
			ComputeUnits:         user.ComputeUnits,
			StorageUnits:         user.StorageUnits,
			NumberSpaces:         usage.NumberSpaces,
			NumberSpacesDeployed: usage.NumberSpacesDeployed,
			UsedComputeUnits:     usage.ComputeUnits,
			UsedStorageUnits:     usage.StorageUnits,
		}

		// Get the groups and build a map
		groups, err := db.GetGroups()
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		groupMap := make(map[string]*model.Group)
		for _, group := range groups {
			groupMap[group.Id] = group
		}

		// Sum the compute and storage units from groups
		for _, groupId := range user.Groups {
			group, ok := groupMap[groupId]
			if ok {
				quota.MaxSpaces += group.MaxSpaces
				quota.ComputeUnits += group.ComputeUnits
				quota.StorageUnits += group.StorageUnits
			}
		}
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

	// Check the quota if this space is started
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		log.Error().Msgf("HandleSpaceStart: get template %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If the user has a compute limit then check it
	if quota.ComputeUnits > 0 && quota.UsedComputeUnits+template.ComputeUnits > quota.ComputeUnits {
		rest.SendJSON(http.StatusInsufficientStorage, w, r, ErrorResponse{Error: "compute unit quota exceeded"})
		return
	}

	// If the user has a storage limit then check it
	if quota.StorageUnits > 0 && quota.UsedStorageUnits+template.StorageUnits > quota.StorageUnits {
		rest.SendJSON(http.StatusInsufficientStorage, w, r, ErrorResponse{Error: "storage unit quota exceeded"})
		return
	}

	// Test if the schedule allows the space to be started
	if !template.AllowedBySchedule() {
		rest.SendJSON(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "outside of schedule"})
		return
	}

	// Mark the space as pending and save it
	space.IsPending = true
	if err = db.SaveSpace(space); err != nil {
		log.Error().Msgf("HandleSpaceStart: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	origin.UpdateSpace(space)

	// Revert the pending status if the deploy fails
	var deployFailed = true
	defer func() {
		if deployFailed {
			// If the deploy failed then revert the space to not pending
			space.IsPending = false
			db.SaveSpace(space)

			// If remote then need to update the remote
			origin.UpdateSpace(space)
		}
	}()

	// Get the variables
	variables, err := db.GetTemplateVars()
	if err != nil {
		log.Error().Msgf("HandleSpaceStart: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	vars := model.FilterVars(variables)

	var containerClient container.ContainerManager
	if template.LocalContainer {
		containerClient = docker.NewClient()
	} else {
		containerClient = nomad.NewClient()
	}

	// Create volumes
	err = containerClient.CreateSpaceVolumes(user, template, space, &vars)
	if err != nil {
		log.Error().Msgf("HandleSpaceStart: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Start the job
	err = containerClient.CreateSpaceJob(user, template, space, &vars)
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
	var space *model.Space

	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	if !validate.UUID(spaceId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid space ID"})
		return
	}

	db := database.GetInstance()

	space, err = db.GetSpace(spaceId)
	if err != nil {
		log.Error().Msgf("HandleSpaceStop: %s", err.Error())
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If user doesn't have permission to manage spaces and not their space then fail
	if user.Id != space.UserId && user.Id != space.SharedWithUserId && !user.HasPermission(model.PermissionManageSpaces) {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	// If the space is not running or changing state then fail
	if (!space.IsDeployed && !space.IsPending) || space.IsDeleting {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "space cannot be stopped"})
		return
	}

	err = deleteSpaceJob(space)
	if err != nil {
		log.Error().Msgf("HandleSpaceStop: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func deleteSpaceJob(space *model.Space) error {
	db := database.GetInstance()

	// Get the template
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		log.Error().Msgf("DeleteSpaceJob: failed to get template %s", err.Error())
		return err
	}

	// Mark the space as pending and save it
	space.IsPending = true
	if err = db.SaveSpace(space); err != nil {
		log.Error().Msgf("DeleteSpaceJob: failed to save space %s", err.Error())
		return err
	}

	origin.UpdateSpace(space)

	var containerClient container.ContainerManager
	if template.LocalContainer {
		containerClient = docker.NewClient()
	} else {
		containerClient = nomad.NewClient()
	}

	// Stop the job
	err = containerClient.DeleteSpaceJob(space)
	if err != nil {
		space.IsPending = false
		db.SaveSpace(space)
		origin.UpdateSpace(space)

		log.Error().Msgf("DeleteSpaceJob: failed to delete space %s", err.Error())
		return err
	}

	return nil
}

func HandleUpdateSpace(w http.ResponseWriter, r *http.Request) {
	var user *model.User = nil
	spaceId := r.PathValue("space_id")

	if !validate.UUID(spaceId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid space ID"})
		return
	}

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
	var template *model.Template = nil
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
		template, err = db.GetTemplate(request.TemplateId)
		if err != nil || template == nil {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Unknown template"})
			return
		}

		audit.Log(
			user.Username,
			model.AuditActorTypeUser,
			model.AuditEventSpaceUpdate,
			fmt.Sprintf("Updated space %s", space.Name),
			&map[string]interface{}{
				"agent":           r.UserAgent(),
				"IP":              r.RemoteAddr,
				"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
				"space_id":        space.Id,
				"space_name":      space.Name,
			},
		)
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

	if template != nil && (space.IsDeployed || template.IsManual) {
		// Get the agent state
		agentState := agent_server.GetSession(space.Id)
		if agentState != nil && agentState.SSHPort > 0 {
			agentState.SendUpdateShell(space.Shell)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func HandleSpaceStopUsersSpaces(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	userId := r.PathValue("user_id")

	if !validate.UUID(userId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	// If the user isn't self then check permissions
	if user.Id != userId && !user.HasPermission(model.PermissionManageUsers) {
		rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot stop spaces for another user"})
		return
	}

	db := database.GetInstance()

	// Get the nomad & container clients
	nomadClient := nomad.NewClient()
	containerClient := docker.NewClient()

	// Stop all spaces
	spaces, err := db.GetSpacesForUser(userId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	for _, space := range spaces {
		// We skip spaces that have been shared with the user
		if space.UserId == userId && space.IsDeployed && (space.Location == "" || space.Location == server_info.LeafLocation) {

			// Load the template for the space
			template, err := db.GetTemplate(space.TemplateId)
			if err != nil {
				log.Error().Msgf("HandleSpaceStopUsersSpaces: %s", err.Error())
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}

			// Mark the space as pending and save it
			space.IsPending = true
			if err = db.SaveSpace(space); err != nil {
				log.Error().Msgf("HandleSpaceStart: %s", err.Error())
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}

			origin.UpdateSpace(space)

			if template.LocalContainer {
				err = containerClient.DeleteSpaceJob(space)
			} else {
				err = nomadClient.DeleteSpaceJob(space)
			}
			if err != nil {
				space.IsPending = false
				db.SaveSpace(space)
				origin.UpdateSpace(space)

				log.Error().Msgf("HandleSpaceStopUsersSpaces: %s", err.Error())
				rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func HandleGetSpace(w http.ResponseWriter, r *http.Request) {
	var space *model.Space
	var err error
	var code int

	spaceId := r.PathValue("space_id")

	if !validate.UUID(spaceId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid space ID"})
		return
	}

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
		UserId:     space.UserId,
		TemplateId: space.TemplateId,
		Name:       space.Name,
		Shell:      space.Shell,
		Location:   space.Location,
		AltNames:   space.AltNames,
		IsDeployed: space.IsDeployed,
		IsPending:  space.IsPending,
		IsDeleting: space.IsDeleting,
		VolumeData: space.VolumeData,
	}

	rest.SendJSON(http.StatusOK, w, r, &response)
}

func RealDeleteSpace(space *model.Space) {
	go func() {
		log.Info().Msgf("api: RealDeleteSpace: deleting %s", space.Id)

		db := database.GetInstance()

		template, err := db.GetTemplate(space.TemplateId)
		if err != nil {
			log.Error().Msgf("api: RealDeleteSpace load template %s", err.Error())

			space.IsDeleting = false
			db.SaveSpace(space)
			return
		}

		var containerClient container.ContainerManager
		if template.LocalContainer {
			containerClient = docker.NewClient()
		} else {
			containerClient = nomad.NewClient()
		}

		// Delete volumes on failure we log the error and revert the space to not deleting
		err = containerClient.DeleteSpaceVolumes(space)
		if err != nil {
			log.Error().Msgf("api: RealDeleteSpace %s", err.Error())

			space.IsDeleting = false
			db.SaveSpace(space)
			return
		}

		// Delete the space
		err = db.DeleteSpace(space)
		if err != nil {
			log.Error().Msgf("api: RealDeleteSpace %s", err.Error())

			space.IsDeleting = false
			db.SaveSpace(space)
			return
		}

		// Delete the agent state if present
		agent_server.RemoveSession(space.Id)

		log.Info().Msgf("api: RealDeleteSpace: deleted %s", space.Id)
	}()

	leaf.DeleteSpace(space.Id)
}

func HandleSpaceTransfer(w http.ResponseWriter, r *http.Request) {
	var err error
	var space *model.Space

	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	request := apiclient.SpaceTransferRequest{}
	err = rest.BindJSON(w, r, &request)
	if err != nil {
		log.Error().Msgf("HandleSpaceTransfer: %s", err.Error())
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.UUID(spaceId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid space ID"})
		return
	}

	if !validate.UUID(request.UserId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	db := database.GetInstance()

	space, err = db.GetSpace(spaceId)
	if err != nil {
		log.Error().Msgf("HandleSpaceTransfer: %s", err.Error())
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If user doesn't own the space then 404
	if space.UserId != user.Id {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	// If the space is running or changing state then fail
	if space.IsDeployed || space.IsPending || space.IsDeleting {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "space cannot be transferred at this time"})
		return
	}

	// If the user is transferring to themselves then fail
	if space.UserId == request.UserId {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "cannot transfer to yourself"})
		return
	}

	// Load the new user
	newUser, err := db.GetUser(request.UserId)
	if err != nil {
		log.Error().Msgf("HandleSpaceTransfer: %s", err.Error())
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If user not found or not active then fail
	if newUser == nil || !newUser.Active {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "user not found"})
		return
	}

	// Check the user has space for the transfer
	userQuota, err := database.GetUserQuota(newUser)
	if err != nil {
		log.Error().Msgf("HandleSpaceTransfer: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	userUsage, err := database.GetUserUsage(newUser.Id, "")
	if err != nil {
		log.Error().Msgf("HandleSpaceTransfer: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if userQuota.MaxSpaces > 0 && uint32(userUsage.NumberSpaces) >= userQuota.MaxSpaces {
		rest.SendJSON(http.StatusInsufficientStorage, w, r, ErrorResponse{Error: "space quota exceeded"})
		return
	}

	// Load the template
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		log.Error().Msgf("HandleSpaceTransfer: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Check the storage quota
	if userQuota.StorageUnits > 0 && userUsage.StorageUnits+template.StorageUnits > userQuota.StorageUnits {
		rest.SendJSON(http.StatusInsufficientStorage, w, r, ErrorResponse{Error: "storage unit quota exceeded"})
		return
	}

	// If template has groups then check the user is in one
	if len(template.Groups) > 0 {
		if !newUser.HasAnyGroup(&template.Groups) {
			rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "user does not have permission to use the space template"})
			return
		}
	}

	// If the volume spec references user.username or user.email then fail
	if strings.Contains(template.Volumes, "user.username") || strings.Contains(template.Volumes, "user.email") {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "template volume spec cannot reference user.username or user.email"})
		return
	}

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.TransferSpace(spaceId, request.UserId)
		if err != nil {
			log.Error().Msgf("HandleSpaceTransfer: %s", err.Error())
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {

		// Test if the target user already has a space with the same name
		name := space.Name
		attempt := 1
		for {
			existing, err := db.GetSpaceByName(request.UserId, name)
			if err == nil && existing != nil {
				name = fmt.Sprintf("%s-%d", space.Name, attempt)
				attempt++

				// If we've had 10 attempts then fail
				if attempt > 10 {
					rest.SendJSON(http.StatusConflict, w, r, ErrorResponse{Error: "user already has a space with the same name"})
					return
				}
			} else {
				break
			}
		}

		// Move the space
		space.Name = name
		space.UserId = request.UserId
		err = db.SaveSpace(space)
		if err != nil {
			log.Error().Msgf("HandleSpaceTransfer: %s", err.Error())
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		leaf.UpdateSpace(space)

		audit.Log(
			user.Username,
			model.AuditActorTypeUser,
			model.AuditEventSpaceTransfer,
			fmt.Sprintf("Transfer space %s to user %s", space.Name, request.UserId),
			&map[string]interface{}{
				"agent":           r.UserAgent(),
				"IP":              r.RemoteAddr,
				"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
				"space_id":        space.Id,
				"space_name":      space.Name,
				"user_id":         request.UserId,
			},
		)
	}

	w.WriteHeader(http.StatusOK)
}

func HandleSpaceAddShare(w http.ResponseWriter, r *http.Request) {
	var err error
	var space *model.Space

	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	request := apiclient.SpaceTransferRequest{}
	err = rest.BindJSON(w, r, &request)
	if err != nil {
		log.Error().Msgf("HandleSpaceAddShare: %s", err.Error())
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.UUID(spaceId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid space ID"})
		return
	}

	if !validate.UUID(request.UserId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	db := database.GetInstance()

	space, err = db.GetSpace(spaceId)
	if err != nil {
		log.Error().Msgf("HandleSpaceAddShare: %s", err.Error())
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If user doesn't own the space then 404
	if space.UserId != user.Id {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	// If the space is deleting or changing state then fail
	if space.IsDeleting {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "space cannot be shared at this time"})
		return
	}

	// If the user is sharing with themselves then fail
	if space.UserId == request.UserId {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "cannot share with yourself"})
		return
	}

	// Load the new user
	newUser, err := db.GetUser(request.UserId)
	if err != nil {
		log.Error().Msgf("HandleSpaceAddShare: %s", err.Error())
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If user not found or not active then fail
	if newUser == nil || !newUser.Active {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "user not found"})
		return
	}

	// Share the space
	space.SharedWithUserId = newUser.Id
	err = db.SaveSpace(space)
	if err != nil {
		log.Error().Msgf("HandleSpaceAddShare: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	leaf.UpdateSpace(space)

	api_utils.UpdateSpaceSSHKeys(space, user)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSpaceShare,
		fmt.Sprintf("Shared space %s to user %s", space.Name, request.UserId),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"space_id":        space.Id,
			"space_name":      space.Name,
			"user_id":         request.UserId,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleSpaceRemoveShare(w http.ResponseWriter, r *http.Request) {
	var err error
	var space *model.Space

	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	if !validate.UUID(spaceId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid space ID"})
		return
	}

	db := database.GetInstance()

	space, err = db.GetSpace(spaceId)
	if err != nil {
		log.Error().Msgf("HandleSpaceRemoveShare: %s", err.Error())
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If user doesn't own the space or space not shared with the user then 404
	if space.UserId != user.Id && space.SharedWithUserId != user.Id {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	if space.SharedWithUserId == "" {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "space is not shared"})
		return
	}

	// Unshare the space
	space.SharedWithUserId = ""
	err = db.SaveSpace(space)
	if err != nil {
		log.Error().Msgf("HandleSpaceRemoveShare: %s", err.Error())
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	leaf.UpdateSpace(space)

	api_utils.UpdateSpaceSSHKeys(space, user)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSpaceStopShare,
		fmt.Sprintf("Stop space share %s", space.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"space_id":        space.Id,
			"space_name":      space.Name,
		},
	)

	w.WriteHeader(http.StatusOK)
}
