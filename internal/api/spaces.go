package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/api/api_utils"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/knot/internal/log"
)

func HandleGetSpaces(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	userId := r.URL.Query().Get("user_id")

	spaceData := &apiclient.SpaceInfoList{
		Count:  0,
		Spaces: []apiclient.SpaceInfo{},
	}

	// If user doesn't have permission to manage spaces and filter user ID doesn't match the user return an empty list
	if !user.HasPermission(model.PermissionManageSpaces) && userId != user.Id {
		rest.WriteResponse(http.StatusOK, w, r, spaceData)
		return
	}

	spaceService := service.GetSpaceService()
	spaces, err := spaceService.ListSpaces(service.SpaceListOptions{
		User:           user,
		UserId:         userId,
		IncludeDeleted: false,
		CheckZone:      false, // API doesn't filter by zone
	})
	if err != nil {
		log.WithError(err).Error("HandleGetSpaces:")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Build a json array of space data to return to the client
	cfg := config.GetServerConfig()
	db := database.GetInstance()
	for _, space := range spaces {
		var templateName string

		// Lookup the template
		template, templateErr := db.GetTemplate(space.TemplateId)
		if templateErr != nil {
			templateName = "Unknown"
		} else {
			templateName = template.Name
		}

		s := apiclient.SpaceInfo{}

		s.Id = space.Id
		s.Name = space.Name
		s.Description = space.Description
		s.Note = space.Note
		s.TemplateName = templateName
		s.TemplateId = space.TemplateId
		s.Zone = space.Zone
		s.IsRemote = space.Zone != "" && space.Zone != cfg.Zone
		s.Platform = template.Platform
		s.IconURL = space.IconURL

		// Get node hostname if node_id is set
		if space.NodeId != "" {
			transport := service.GetTransport()
			if transport != nil {
				node := transport.GetNodeByIDString(space.NodeId)
				if node != nil {
					s.NodeHostname = node.Metadata.GetString("hostname")
				}
				if s.NodeHostname == "" {
					s.NodeHostname = "Offline Remote Node"
				}
			} else {
				// Leaf mode - all nodes are local
				s.NodeHostname = cfg.Hostname
			}
		}

		// Get the user
		u, err := db.GetUser(space.UserId)
		if err != nil {
			log.WithError(err).Error("HandleGetSpaces: GetUser:")
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		s.Username = u.Username
		s.UserId = u.Id

		s.Shares = api_utils.BuildAPIShares(space)

		// Get the space state
		s.IsDeployed = space.IsDeployed
		s.IsPending = space.IsPending
		s.IsDeleting = space.IsDeleting
		s.StartedAt = space.StartedAt.UTC()

		state := agent_server.GetSession(space.Id)
		if state == nil {
			s.HasCodeServer = false
			s.HasSSH = false
			s.HasTerminal = false
			s.HasHttpVNC = false
			s.TcpPorts = make(map[string]string)
			s.HttpPorts = make(map[string]string)
			s.HasVSCodeTunnel = false
			s.VSCodeTunnel = ""
			s.HasState = false
		} else {
			s.HasCodeServer = state.HasCodeServer
			s.HasSSH = state.SSHPort > 0
			s.HasTerminal = state.HasTerminal
			s.HasHttpVNC = state.VNCHttpPort > 0
			s.TcpPorts = state.TcpPorts
			s.HasState = true

			// If wildcard domain is set then offer the http ports
			if cfg.WildcardDomain == "" {
				s.HttpPorts = make(map[string]string)
			} else {
				s.HttpPorts = state.HttpPorts
			}

			s.HasVSCodeTunnel = state.HasVSCodeTunnel
			s.VSCodeTunnel = state.VSCodeTunnelName

			// If template is manual then force IsDeployed to true as agent is live
			if template.IsManual() {
				s.IsDeployed = true
			}
		}

		// Check if the template has been updated
		if template.IsManual() || template.Hash == "" {
			s.UpdateAvailable = false
		} else {
			s.UpdateAvailable = space.IsDeployed && space.TemplateHash != template.Hash
		}

		spaceData.Spaces = append(spaceData.Spaces, s)
		spaceData.Count++
	}

	rest.WriteResponse(http.StatusOK, w, r, spaceData)
}

func HandleDeleteSpace(w http.ResponseWriter, r *http.Request) {
	var user *model.User = nil
	spaceId := r.PathValue("space_id")

	db := database.GetInstance()

	if r.Context().Value("user") != nil {
		user = r.Context().Value("user").(*model.User)
	}

	// Support lookup by both ID and name
	var space *model.Space
	var err error
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: fmt.Sprintf("space %s not found", spaceId)})
		return
	}
	spaceName := space.Name
	spaceId = space.Id // Use the resolved ID for subsequent operations

	// Check if request should be forwarded to another node
	if shouldForward, nodeId := service.ShouldForwardToNode(space.NodeId); shouldForward {
		if err := service.ForwardToNode(w, r, nodeId); err != nil {
			// If forwarding fails, allow delete to proceed (node might be dead)
			log.WithError(err).Warn("failed to forward delete request, proceeding locally")
		} else {
			return
		}
	}

	// API-specific logic for checking if space can be deleted
	cfg := config.GetServerConfig()
	if space.IsDeployed || space.IsPending || space.IsDeleting || (space.Zone != "" && space.Zone != cfg.Zone) {
		rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: "space cannot be deleted"})
		return
	}

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSpaceDelete,
		fmt.Sprintf("Deleted space %s", spaceName),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"space_id":        spaceId,
			"space_name":      spaceName,
		},
	)

	// Mark the space as deleting and delete it in the background (API-specific logic)
	space.IsDeleting = true
	space.UpdatedAt = hlc.Now()
	db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
	service.GetTransport().GossipSpace(space)

	// Delete the space in the background
	service.GetContainerService().DeleteSpace(space)

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
	request := apiclient.SpaceRequest{}
	user := r.Context().Value("user").(*model.User)

	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		log.WithError(err).Error("HandleCreateSpace:")
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Remove any blank alt names, any that match the primary name, and any duplicates
	request.AltNames = removeBlankAndDuplicates(request.AltNames, request.Name)

	// If user give and not our ID and no permission to manage spaces then fail
	if request.UserId != "" && request.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot create space for another user"})
		return
	}

	// If creating for another user, get that user
	if request.UserId != "" {
		db := database.GetInstance()
		var err error
		user, err = db.GetUser(request.UserId)
		if err != nil {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// Convert custom fields
	var customFields []model.SpaceCustomField
	for _, field := range request.CustomFields {
		customFields = append(customFields, model.SpaceCustomField{
			Name:  field.Name,
			Value: field.Value,
		})
	}

	// Get template for node selection
	db := database.GetInstance()
	template, err := db.GetTemplate(request.TemplateId)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "template not found"})
		return
	}

	// Select node for space
	nodeId, err := service.SelectNodeForSpace(template, request.SelectedNodeId)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Create the space
	space := model.NewSpace(request.Name, request.Description, user.Id, request.TemplateId, request.Shell, &request.AltNames, "", request.IconURL, customFields)
	space.NodeId = nodeId
	space.StartupScriptId = request.StartupScriptId

	spaceService := service.GetSpaceService()
	err = spaceService.CreateSpace(space, user)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

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

	// Return the Token ID
	rest.WriteResponse(http.StatusCreated, w, r, struct {
		Status  bool   `json:"status"`
		SpaceID string `json:"space_id"`
	}{
		Status:  true,
		SpaceID: space.Id,
	})
}

func HandleSpaceStart(w http.ResponseWriter, r *http.Request) {
	logger := log.WithGroup("server")

	var err error
	var space *model.Space

	spaceId := r.PathValue("space_id")

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()
	cfg := config.GetServerConfig()

	// Support lookup by both ID and name
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		logger.WithError(err).Error("get space failed")
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	spaceId = space.Id // Use the resolved ID for subsequent operations

	// Check if request should be forwarded to another node
	if shouldForward, nodeId := service.ShouldForwardToNode(space.NodeId); shouldForward {
		if err := service.ForwardToNode(w, r, nodeId); err != nil {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to forward request"})
		}
		return
	}

	// Acquire lock after forwarding check
	transport := service.GetTransport()
	unlockToken := transport.LockResource(spaceId)
	if unlockToken == "" {
		logger.Error("failed to lock space")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to lock space"})
		return
	}
	defer transport.UnlockResource(spaceId, unlockToken)
	if err != nil {
		logger.WithError(err).Error("get space failed")
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If user doesn't have permission to manage spaces and not their space then fail
	if user.Id != space.UserId && !space.IsSharedWith(user.Id) && !user.HasPermission(model.PermissionManageSpaces) {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	// If the space is owned by a different user then load the user
	if user.Id != space.UserId {
		user, err = db.GetUser(space.UserId)
		if err != nil {
			logger.WithError(err).Error("get user failed")
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// If the space is already running or changing state then fail
	if space.IsDeployed || space.IsPending || space.IsDeleting {
		rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: "space cannot be started"})
		return
	}

	// Is the space has a zone then it must match the server zone
	if space.Zone != "" && space.Zone != cfg.Zone {
		rest.WriteResponse(http.StatusNotAcceptable, w, r, ErrorResponse{Error: "space zone does not match server zone"})
		return
	}

	// Check the quota if this space is started
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		logger.WithError(err).Error("get template")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !cfg.LeafNode {
		usage, err := database.GetUserUsage(user.Id, "")
		if err != nil {
			logger.WithError(err).Error("get user usage failed")
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		userQuota, err := database.GetUserQuota(user)
		if err != nil {
			logger.WithError(err).Error("get user quota failed")
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		if userQuota.ComputeUnits > 0 && usage.ComputeUnits+template.ComputeUnits > userQuota.ComputeUnits {
			rest.WriteResponse(http.StatusInsufficientStorage, w, r, ErrorResponse{Error: "compute unit quota exceeded"})
			return
		}
	}

	// Test if the schedule allows the space to be started
	if !template.AllowedBySchedule() {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "outside of schedule"})
		return
	}

	if err := service.GetContainerService().StartSpace(space, template, user); err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleSpaceStop(w http.ResponseWriter, r *http.Request) {
	var err error
	var space *model.Space

	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	db := database.GetInstance()
	cfg := config.GetServerConfig()

	// Support lookup by both ID and name
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		log.WithError(err).Error("HandleSpaceStop:")
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	spaceId = space.Id // Use the resolved ID for subsequent operations

	// Check if request should be forwarded to another node
	if shouldForward, nodeId := service.ShouldForwardToNode(space.NodeId); shouldForward {
		if err := service.ForwardToNode(w, r, nodeId); err != nil {
			// If forwarding fails, allow stop to proceed (node might be dead)
			log.WithError(err).Warn("failed to forward stop request, proceeding locally")
		}
		return
	}

	// If user doesn't have permission to manage spaces and not their space then fail
	if user.Id != space.UserId && !space.IsSharedWith(user.Id) && !user.HasPermission(model.PermissionManageSpaces) {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	// If the space is not running or changing state then fail
	if (!space.IsDeployed && !space.IsPending) || space.IsDeleting {
		rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: "space cannot be stopped"})
		return
	}

	// If the space isn't on this server then fail
	if space.Zone != "" && space.Zone != cfg.Zone {
		rest.WriteResponse(http.StatusNotAcceptable, w, r, ErrorResponse{Error: "space not on this server"})
		return
	}

	err = service.GetContainerService().StopSpace(space)
	if err != nil {
		log.WithError(err).Error("HandleSpaceStop:")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleSpaceRestart(w http.ResponseWriter, r *http.Request) {
	var err error
	var space *model.Space

	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	db := database.GetInstance()
	cfg := config.GetServerConfig()

	// Support lookup by both ID and name
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		log.WithError(err).Error("HandleSpaceRestart")
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	spaceId = space.Id // Use the resolved ID for subsequent operations

	// Check if request should be forwarded to another node
	if shouldForward, nodeId := service.ShouldForwardToNode(space.NodeId); shouldForward {
		if err := service.ForwardToNode(w, r, nodeId); err != nil {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to forward request"})
		}
		return
	}

	// If user doesn't have permission to manage spaces and not their space then fail
	if user.Id != space.UserId && !space.IsSharedWith(user.Id) && !user.HasPermission(model.PermissionManageSpaces) {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	// If the space is not running or changing state then fail
	if (!space.IsDeployed && !space.IsPending) || space.IsDeleting {
		rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: "space cannot be restarted"})
		return
	}

	// If the space isn't on this server then fail
	if space.Zone != "" && space.Zone != cfg.Zone {
		rest.WriteResponse(http.StatusNotAcceptable, w, r, ErrorResponse{Error: "space not on this server"})
		return
	}

	err = service.GetContainerService().RestartSpace(space)
	if err != nil {
		log.WithError(err).Error("HandleSpaceRestart:")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleUpdateSpace(w http.ResponseWriter, r *http.Request) {
	var user *model.User = nil
	spaceId := r.PathValue("space_id")

	if r.Context().Value("user") != nil {
		user = r.Context().Value("user").(*model.User)
	}

	db := database.GetInstance()
	// Support lookup by both ID and name
	var space *model.Space
	var err error
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		log.WithError(err).Error("HandleUpdateSpace:")
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	spaceId = space.Id // Use the resolved ID for subsequent operations

	spaceService := service.GetSpaceService()
	request := apiclient.SpaceRequest{}
	err = rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		log.WithError(err).Error("HandleUpdateSpace:")
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Remove any blank alt names, any that match the primary name, and any duplicates
	request.AltNames = removeBlankAndDuplicates(request.AltNames, request.Name)

	// Convert custom fields
	var customFields []model.SpaceCustomField
	for _, field := range request.CustomFields {
		customFields = append(customFields, model.SpaceCustomField{
			Name:  field.Name,
			Value: field.Value,
		})
	}

	// Update the space with request data
	space.Name = request.Name
	space.Description = request.Description
	space.TemplateId = request.TemplateId
	space.Shell = request.Shell
	space.AltNames = request.AltNames
	space.IconURL = request.IconURL
	space.CustomFields = customFields
	space.StartupScriptId = request.StartupScriptId

	err = spaceService.UpdateSpace(space, user)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
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

	// API-specific logic for updating shell on deployed spaces
	template, templateErr := db.GetTemplate(space.TemplateId)
	if templateErr == nil && template != nil && (space.IsDeployed || template.IsManual()) {
		// Get the agent state
		agentState := agent_server.GetSession(space.Id)
		if agentState != nil && agentState.SSHPort > 0 {
			agentState.SendUpdateShell(space.Shell)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func HandleSetSpaceCustomField(w http.ResponseWriter, r *http.Request) {
	var user *model.User = nil
	spaceId := r.PathValue("space_id")

	if r.Context().Value("user") != nil {
		user = r.Context().Value("user").(*model.User)
	}

	request := apiclient.SetCustomFieldRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		log.WithError(err).Error("HandleSetSpaceCustomField:")
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Support lookup by both ID and name
	db := database.GetInstance()
	var space *model.Space
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		log.WithError(err).Error("HandleSetSpaceCustomField:")
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	spaceId = space.Id // Use the resolved ID for subsequent operations

	spaceService := service.GetSpaceService()
	err = spaceService.SetSpaceCustomField(spaceId, request.Name, request.Value, user)
	if err != nil {
		log.WithError(err).Error("HandleSetSpaceCustomField:")
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Use the space we already looked up for audit logging
	spaceName := space.Name

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSpaceUpdate,
		fmt.Sprintf("Set custom field '%s' on space %s", request.Name, spaceName),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"space_id":        spaceId,
			"space_name":      spaceName,
			"field_name":      request.Name,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleGetSpaceCustomField(w http.ResponseWriter, r *http.Request) {
	var user *model.User = nil
	spaceId := r.PathValue("space_id")
	fieldName := r.PathValue("field_name")

	if fieldName == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Field name is required"})
		return
	}

	if r.Context().Value("user") != nil {
		user = r.Context().Value("user").(*model.User)
	}

	// Support lookup by both ID and name
	db := database.GetInstance()
	var space *model.Space
	var err error
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		log.WithError(err).Error("HandleGetSpaceCustomField:")
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	spaceId = space.Id // Use the resolved ID for subsequent operations

	spaceService := service.GetSpaceService()
	value, err := spaceService.GetSpaceCustomField(spaceId, fieldName, user)
	if err != nil {
		log.WithError(err).Error("HandleGetSpaceCustomField:")
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, apiclient.GetCustomFieldResponse{
		Name:  fieldName,
		Value: value,
	})
}

func HandleSpaceStopUsersSpaces(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	userId := r.PathValue("user_id")

	db := database.GetInstance()

	// Support lookup by both ID and username
	var targetUser *model.User
	var err error
	if validate.UUID(userId) {
		targetUser, err = db.GetUser(userId)
	} else {
		targetUser, err = db.GetUserByUsername(userId)
	}
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "User not found"})
		return
	}
	userId = targetUser.Id // Use the resolved ID for subsequent operations

	// If the user isn't self then check permissions
	if user.Id != userId && !user.HasPermission(model.PermissionManageUsers) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot stop spaces for another user"})
		return
	}

	// Stop all spaces
	spaces, err := db.GetSpacesForUser(userId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	cfg := config.GetServerConfig()
	for _, space := range spaces {
		// We skip spaces that have been shared with the user
		if space.UserId == userId && space.IsDeployed && (space.Zone == "" || space.Zone == cfg.Zone) {
			if err := service.GetContainerService().StopSpace(space); err != nil {
				rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
				return
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func HandleGetSpace(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	// Support lookup by both ID and name
	var space *model.Space
	var err error
	db := database.GetInstance()

	if validate.UUID(spaceId) {
		// Lookup by ID
		space, err = db.GetSpace(spaceId)
	} else {
		// Lookup by name
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}

	if err != nil || space == nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	// Now use GetSpaceDetails with the resolved ID
	data, err := api_utils.GetSpaceDetails(space.Id, user)
	if err != nil {
		if err.Error() == "Space not found: sql: no rows in result set" {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Space not found"})
		} else if err.Error() == "No permission to access this space" {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		} else {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		}
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, data)
}

func HandleSpaceTransfer(w http.ResponseWriter, r *http.Request) {
	var err error
	var space *model.Space

	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	request := apiclient.SpaceShareUpdateRequest{}
	err = rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		log.WithError(err).Error("HandleSpaceTransfer:")
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	requestedShares := request.Shares
	if len(requestedShares) == 0 && request.UserId != "" {
		requestedShares = []apiclient.SpaceShareRequest{{
			UserId:     request.UserId,
			Permission: "full",
		}}
	}
	if len(requestedShares) == 0 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "at least one share is required"})
		return
	}
	if len(requestedShares) > 1 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "multiple shares require pro"})
		return
	}
	if requestedShares[0].Permission == "" {
		requestedShares[0].Permission = "full"
	}

	db := database.GetInstance()

	// Resolve user_id to UUID (supports UUID, username, or email)
	var targetUserId string
	requestedUserId := requestedShares[0].UserId
	if validate.UUID(requestedUserId) {
		targetUserId = requestedUserId
	} else if strings.Contains(requestedUserId, "@") {
		// Lookup by email
		targetUser, err := db.GetUserByEmail(requestedUserId)
		if err != nil || targetUser == nil {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: fmt.Sprintf("user '%s' not found", requestedUserId)})
			return
		}
		targetUserId = targetUser.Id
	} else {
		// Lookup by username
		targetUser, err := db.GetUserByUsername(requestedUserId)
		if err != nil || targetUser == nil {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: fmt.Sprintf("user '%s' not found", requestedUserId)})
			return
		}
		targetUserId = targetUser.Id
	}

	// Support lookup by both ID and name
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		log.WithError(err).Error("HandleSpaceTransfer:")
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	spaceId = space.Id // Use the resolved ID for subsequent operations

	// If user doesn't own the space and doesn't have transfer permission then 404
	if space.UserId != user.Id && !user.HasPermission(model.PermissionTransferSpaces) {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	// If space isn't on this server then fail
	cfg := config.GetServerConfig()
	if space.Zone != "" && space.Zone != cfg.Zone {
		rest.WriteResponse(http.StatusNotAcceptable, w, r, ErrorResponse{Error: "space not on this server"})
		return
	}

	// If the space is running or changing state then fail
	if space.IsDeployed || space.IsPending || space.IsDeleting {
		rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: "space cannot be transferred at this time"})
		return
	}

	// If the user is transferring to themselves then fail
	if space.UserId == targetUserId {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "cannot transfer to yourself"})
		return
	}

	// Load the new user
	newUser, err := db.GetUser(targetUserId)
	if err != nil {
		log.WithError(err).Error("HandleSpaceTransfer:")
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If user not found or not active then fail
	if newUser == nil || !newUser.Active || newUser.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "user not found"})
		return
	}

	// Check the user has space for the transfer
	userQuota, err := database.GetUserQuota(newUser)
	if err != nil {
		log.WithError(err).Error("HandleSpaceTransfer:")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	userUsage, err := database.GetUserUsage(newUser.Id, "")
	if err != nil {
		log.WithError(err).Error("HandleSpaceTransfer:")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if userQuota.MaxSpaces > 0 && uint32(userUsage.NumberSpaces) >= userQuota.MaxSpaces {
		rest.WriteResponse(http.StatusInsufficientStorage, w, r, ErrorResponse{Error: "space quota exceeded"})
		return
	}

	// Load the template
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		log.WithError(err).Error("HandleSpaceTransfer:")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Check the storage quota
	if userQuota.StorageUnits > 0 && userUsage.StorageUnits+template.StorageUnits > userQuota.StorageUnits {
		rest.WriteResponse(http.StatusInsufficientStorage, w, r, ErrorResponse{Error: "storage unit quota exceeded"})
		return
	}

	// If template has groups then check the user is in one or is an admin
	if len(template.Groups) > 0 && !newUser.IsAdmin() {
		if !newUser.HasAnyGroup(&template.Groups) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "user does not have permission to use the space template"})
			return
		}
	}

	// If the volume spec references user.username or user.email then fail
	if strings.Contains(template.Volumes, "user.username") || strings.Contains(template.Volumes, "user.email") {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "template volume spec cannot reference user.username or user.email"})
		return
	}

	// Test if the target user already has a space with the same name
	name := space.Name
	attempt := 1
	for {
		existing, err := db.GetSpaceByName(targetUserId, name)
		if err == nil && existing != nil {
			name = fmt.Sprintf("%s-%d", space.Name, attempt)
			attempt++

			// If we've had 10 attempts then fail
			if attempt > 10 {
				rest.WriteResponse(http.StatusConflict, w, r, ErrorResponse{Error: "user already has a space with the same name"})
				return
			}

			continue
		}

		// Move the space
		space.Name = name
		space.UserId = targetUserId
		space.UpdatedAt = hlc.Now()
		err = db.SaveSpace(space, []string{"Name", "UserId", "UpdatedAt"})
		if err != nil {
			log.WithError(err).Error("HandleSpaceTransfer:")
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		service.GetTransport().GossipSpace(space)

		// Publish SSE event with both old and new user IDs
		sse.PublishSpaceChanged(space.Id, space.UserId, "", user.Id)

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

		break
	}

	w.WriteHeader(http.StatusOK)
}

func HandleSpaceAddShare(w http.ResponseWriter, r *http.Request) {
	var err error
	var space *model.Space

	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	request := apiclient.SpaceShareUpdateRequest{}
	err = rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		log.WithError(err).Error("HandleSpaceAddShare:")
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	requestedShares := request.Shares
	if len(requestedShares) == 0 && request.UserId != "" {
		requestedShares = []apiclient.SpaceShareRequest{{
			UserId:     request.UserId,
			Permission: "full",
		}}
	}
	if len(requestedShares) == 0 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "at least one share is required"})
		return
	}
	if len(requestedShares) > 1 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "multiple shares require pro"})
		return
	}
	if requestedShares[0].Permission == "" {
		requestedShares[0].Permission = "full"
	}

	db := database.GetInstance()

	// Resolve user_id to UUID (supports UUID, username, or email)
	var targetUserId string
	requestedUserId := requestedShares[0].UserId
	if validate.UUID(requestedUserId) {
		targetUserId = requestedUserId
	} else if strings.Contains(requestedUserId, "@") {
		// Lookup by email
		targetUser, err := db.GetUserByEmail(requestedUserId)
		if err != nil || targetUser == nil {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: fmt.Sprintf("user '%s' not found", requestedUserId)})
			return
		}
		targetUserId = targetUser.Id
	} else {
		// Lookup by username
		targetUser, err := db.GetUserByUsername(requestedUserId)
		if err != nil || targetUser == nil {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: fmt.Sprintf("user '%s' not found", requestedUserId)})
			return
		}
		targetUserId = targetUser.Id
	}

	// Support lookup by both ID and name
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		log.WithError(err).Error("HandleSpaceAddShare:")
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	spaceId = space.Id // Use the resolved ID for subsequent operations

	// If user doesn't own the space then 404
	if space.UserId != user.Id {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	// If space isn't on this server then fail
	cfg := config.GetServerConfig()
	if space.Zone != "" && space.Zone != cfg.Zone {
		rest.WriteResponse(http.StatusNotAcceptable, w, r, ErrorResponse{Error: "space not on this server"})
		return
	}

	// If the space is deleting or changing state then fail
	if space.IsDeleting {
		rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: "space cannot be shared at this time"})
		return
	}

	// If the user is sharing with themselves then fail
	if space.UserId == targetUserId {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "cannot share with yourself"})
		return
	}

	// Load the new user
	newUser, err := db.GetUser(targetUserId)
	if err != nil {
		log.WithError(err).Error("HandleSpaceAddShare:")
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If user not found or not active then fail
	if newUser == nil || !newUser.Active {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "user not found"})
		return
	}

	// Share the space
	space.SetSingleShare(newUser.Id)
	space.UpdatedAt = hlc.Now()
	err = db.SaveSpace(space, []string{"Shares", "UpdatedAt"})
	if err != nil {
		log.WithError(err).Error("HandleSpaceAddShare:")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipSpace(space)
	service.GetUserService().UpdateSpaceSSHKeys(space, user)

	// Publish SSE event for both owner and shared user
	sse.PublishSpaceChanged(space.Id, space.UserId, space.SharedWithUserId)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSpaceShare,
		fmt.Sprintf("Shared space %s to user %s", space.Name, requestedUserId),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"space_id":        space.Id,
			"space_name":      space.Name,
			"user_id":         requestedUserId,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleSpaceRemoveShare(w http.ResponseWriter, r *http.Request) {
	var err error
	var space *model.Space

	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	db := database.GetInstance()

	// Support lookup by both ID and name
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		log.WithError(err).Error("HandleSpaceRemoveShare:")
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	spaceId = space.Id // Use the resolved ID for subsequent operations

	// If user doesn't own the space or space not shared with the user then 404
	if space.UserId != user.Id && !space.IsSharedWith(user.Id) {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	// If space isn't on this server then fail
	cfg := config.GetServerConfig()
	if space.Zone != "" && space.Zone != cfg.Zone {
		rest.WriteResponse(http.StatusNotAcceptable, w, r, ErrorResponse{Error: "space not on this server"})
		return
	}

	if len(space.SharedUserIds()) == 0 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "space is not shared"})
		return
	}

	// Store the previously shared user ID before clearing
	previousSharedUserId := space.SharedWithUserId

	// Unshare the space
	space.SetSingleShare("")
	space.UpdatedAt = hlc.Now()
	err = db.SaveSpace(space, []string{"Shares", "UpdatedAt"})
	if err != nil {
		log.WithError(err).Error("HandleSpaceRemoveShare:")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipSpace(space)
	service.GetUserService().UpdateSpaceSSHKeys(space, user)

	// Publish SSE event with previous shared user ID so they can remove it from their list
	sse.PublishSpaceChanged(space.Id, space.UserId, "", previousSharedUserId)

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
