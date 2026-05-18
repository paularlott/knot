package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/gossip"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container/runtime"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/specvalidate"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleGetVolumes(w http.ResponseWriter, r *http.Request) {
	volumes, err := database.GetInstance().GetVolumes()
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Build a json array of data to return to the client
	volumeData := apiclient.VolumeInfoList{
		Count:   0,
		Volumes: []apiclient.VolumeInfo{},
	}

	for _, volume := range volumes {
		if volume.IsDeleted {
			continue
		}

		v := apiclient.VolumeInfo{
			Id:       volume.Id,
			Name:     volume.Name,
			Active:   volume.Active,
			Zone:     volume.Zone,
			NodeId:   volume.NodeId,
			Platform: volume.Platform,
		}

		// Resolve node hostname
		if volume.NodeId != "" {
			transport := service.GetTransport()
			if transport != nil {
				node := transport.GetNodeByIDString(volume.NodeId)
				if node != nil {
					v.NodeHostname = node.Metadata.GetString("hostname")
				}
			}
			if v.NodeHostname == "" {
				v.NodeHostname = config.GetServerConfig().Hostname
			}
		}

		volumeData.Volumes = append(volumeData.Volumes, v)
		volumeData.Count++
	}

	rest.WriteResponse(http.StatusOK, w, r, volumeData)
}

func HandleUpdateVolume(w http.ResponseWriter, r *http.Request) {
	volumeId := r.PathValue("volume_id")

	// Support lookup by both ID and name
	db := database.GetInstance()
	var volume *model.Volume
	var err error
	if validate.UUID(volumeId) {
		// Lookup by ID
		volume, err = db.GetVolume(volumeId)
	} else {
		// Lookup by name
		volume, err = db.GetVolumeByName(volumeId)
	}

	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "volume not found"})
		return
	}

	request := apiclient.VolumeUpdateRequest{}
	err = rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume name given"})
		return
	}
	if !validate.MaxLength(request.Definition, 10*1024*1024) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Volume definition must be less than 10MB"})
		return
	}
	if !validate.OneOf(request.Platform, []string{model.PlatformDocker, model.PlatformPodman, model.PlatformNomad, model.PlatformApple, model.PlatformContainer}) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid platform name given"})
		return
	}

	user := r.Context().Value("user").(*model.User)

	// If the volume is active then don't update
	if volume.Active {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Cannot update an active volume"})
		return
	}

	volume.Name = request.Name
	volume.Definition = request.Definition
	volume.UpdatedUserId = user.Id
	volume.Platform = request.Platform
	volume.NodeId = request.NodeId
	volume.UpdatedAt = hlc.Now()

	err = db.SaveVolume(volume, []string{"Name", "Definition", "UpdatedUserId", "Platform", "NodeId", "UpdatedAt"})
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipVolume(volume)
	sse.PublishVolumesChanged(volume.Id)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventVolumeUpdate,
		fmt.Sprintf("Updated volume %s", volume.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"volume_id":       volume.Id,
			"volume_name":     volume.Name,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleCreateVolume(w http.ResponseWriter, r *http.Request) {
	request := apiclient.VolumeCreateRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume name given"})
		return
	}
	if !validate.MaxLength(request.Definition, 10*1024*1024) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Volume definition must be less than 10MB"})
		return
	}
	if !validate.OneOf(request.Platform, []string{model.PlatformDocker, model.PlatformPodman, model.PlatformNomad, model.PlatformApple, model.PlatformContainer}) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid platform name given"})
		return
	}

	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)

	volume := model.NewVolume(request.Name, request.Definition, user.Id, request.Platform)
	volume.NodeId = request.NodeId

	err = db.SaveVolume(volume, nil)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipVolume(volume)
	sse.PublishVolumesChanged(volume.Id)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventVolumeCreate,
		fmt.Sprintf("Created volume %s", volume.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"volume_id":       volume.Id,
			"volume_name":     volume.Name,
		},
	)

	// Return the ID
	rest.WriteResponse(http.StatusCreated, w, r, &apiclient.VolumeCreateResponse{
		Status:   true,
		VolumeId: volume.Id,
	})
}

func HandleValidateVolume(w http.ResponseWriter, r *http.Request) {
	request := apiclient.VolumeValidateRequest{}
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	issues := specvalidate.ValidateVolumeSpec(request.Platform, request.Definition)
	response := apiclient.ValidationResponse{
		Valid:  len(issues) == 0,
		Errors: make([]apiclient.ValidationError, 0, len(issues)),
	}

	for _, issue := range issues {
		response.Errors = append(response.Errors, apiclient.ValidationError{
			Field:   issue.Field,
			Message: issue.Message,
		})
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func HandleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	volumeId := r.PathValue("volume_id")

	// Support lookup by both ID and name
	db := database.GetInstance()
	var volume *model.Volume
	var err error
	if validate.UUID(volumeId) {
		// Lookup by ID
		volume, err = db.GetVolume(volumeId)
	} else {
		// Lookup by name
		volume, err = db.GetVolumeByName(volumeId)
	}

	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "volume not found"})
		return
	}

	// If the volume is active then don't delete
	if volume.Active {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Cannot delete an active volume"})
		return
	}

	// Delete the volume
	volume.IsDeleted = true
	volume.UpdatedAt = hlc.Now()
	volume.UpdatedUserId = r.Context().Value("user").(*model.User).Id
	err = db.SaveVolume(volume, []string{"IsDeleted", "UpdatedAt", "UpdatedUserId"})
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipVolume(volume)
	sse.PublishVolumesDeleted(volume.Id)

	user := r.Context().Value("user").(*model.User)
	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventVolumeDelete,
		fmt.Sprintf("Deleted volume %s", volume.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"volume_id":       volume.Id,
			"volume_name":     volume.Name,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleGetVolume(w http.ResponseWriter, r *http.Request) {
	volumeId := r.PathValue("volume_id")

	// Support lookup by both ID and name
	var volume *model.Volume
	var err error
	db := database.GetInstance()

	if validate.UUID(volumeId) {
		// Lookup by ID
		volume, err = db.GetVolume(volumeId)
	} else {
		// Lookup by name
		volume, err = db.GetVolumeByName(volumeId)
	}

	if err != nil || volume == nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "volume not found"})
		return
	}

	data := apiclient.VolumeDefinition{
		VolumeId:   volume.Id,
		Name:       volume.Name,
		Definition: volume.Definition,
		Active:     volume.Active,
		Zone:       volume.Zone,
		NodeId:     volume.NodeId,
		Platform:   volume.Platform,
	}

	if volume.NodeId != "" {
		transport := service.GetTransport()
		if transport != nil {
			if node := transport.GetNodeByIDString(volume.NodeId); node != nil {
				data.NodeHostname = node.Metadata.GetString("hostname")
			}
		}
		if data.NodeHostname == "" {
			data.NodeHostname = config.GetServerConfig().Hostname
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, &data)
}

func HandleGetVolumeNodes(w http.ResponseWriter, r *http.Request) {
	platform := r.URL.Query().Get("platform")
	if platform == "" || platform == model.PlatformNomad {
		rest.WriteResponse(http.StatusOK, w, r, []AvailableNode{})
		return
	}

	// Only local container platforms need node selection
	isLocal := platform == model.PlatformDocker || platform == model.PlatformPodman ||
		platform == model.PlatformApple || platform == model.PlatformContainer
	if !isLocal {
		rest.WriteResponse(http.StatusOK, w, r, []AvailableNode{})
		return
	}

	cfg := config.GetServerConfig()
	db := database.GetInstance()
	transport := service.GetTransport()

	nodeIdCfg, _ := db.GetCfgValue("node_id")
	localNodeId := ""
	if nodeIdCfg != nil {
		localNodeId = nodeIdCfg.Value
	}

	// Build a fake template to reuse hasRequiredRuntime
	fakeTemplate := &model.Template{Platform: platform}

	var nodes []AvailableNode
	peers := transport.Nodes()

	if !cfg.LeafNode {
		if peers == nil {
			if hasRequiredRuntime(fakeTemplate, runtime.DetectAllAvailableRuntimes(cfg.LocalContainerRuntimePref)) {
				nodes = append(nodes, AvailableNode{
					NodeId:   localNodeId,
					Hostname: cfg.Hostname,
				})
			}
		} else {
			for _, peer := range peers {
				if peer.Metadata.GetString("zone") != cfg.Zone {
					continue
				}
				if peer.GetObservedState() != gossip.NodeAlive {
					continue
				}

				nodeId := peer.ID.String()
				var runtimes []string
				var hostname string
				if nodeId == localNodeId {
					runtimes = runtime.DetectAllAvailableRuntimes(cfg.LocalContainerRuntimePref)
					hostname = cfg.Hostname
				} else {
					runtimes = strings.Split(peer.Metadata.GetString("runtimes"), ",")
					hostname = peer.Metadata.GetString("hostname")
				}

				if hasRequiredRuntime(fakeTemplate, runtimes) {
					nodes = append(nodes, AvailableNode{
						NodeId:   nodeId,
						Hostname: hostname,
					})
				}
			}
		}
	} else {
		nodes = []AvailableNode{}
	}

	rest.WriteResponse(http.StatusOK, w, r, nodes)
}

func HandleVolumeStart(w http.ResponseWriter, r *http.Request) {
	var volume *model.Volume
	var err error

	db := database.GetInstance()

	volumeId := r.PathValue("volume_id")

	// Support lookup by both ID and name
	if validate.UUID(volumeId) {
		volume, err = db.GetVolume(volumeId)
	} else {
		volume, err = db.GetVolumeByName(volumeId)
	}

	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "volume not found"})
		return
	}

	// For local container platforms, resolve the node to run on
	if volume.Platform != model.PlatformNomad {
		if volume.NodeId == "" {
			fakeTemplate := &model.Template{Platform: volume.Platform}
			nodeId, err := service.SelectNodeForSpace(fakeTemplate, "")
			if err != nil {
				rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
				return
			}
			volume.NodeId = nodeId
		}
		if shouldForward, nodeId := service.ShouldForwardToNode(volume.NodeId); shouldForward {
			if err := service.ForwardToNode(w, r, nodeId); err != nil {
				rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to forward request"})
			}
			return
		}
	}

	// Capture resolved NodeId — the DB reload below will overwrite it with the stored (possibly empty) value
	resolvedNodeId := volume.NodeId

	transport := service.GetTransport()
	unlockToken := transport.LockResource(volume.Id)
	if unlockToken == "" {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to lock volume"})
		return
	}
	defer transport.UnlockResource(volume.Id, unlockToken)

	volume, err = db.GetVolume(volume.Id)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Restore the resolved NodeId after reload
	if resolvedNodeId != "" {
		volume.NodeId = resolvedNodeId
	}

	if volume.Active {
		rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: "volume is running"})
		return
	}

	cfg := config.GetServerConfig()
	if volume.Zone != "" && volume.Zone != cfg.Zone {
		rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: "volume is used by another server"})
		return
	}

	err = service.GetContainerService().CreateVolume(volume)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	volume.Zone = cfg.Zone
	volume.Active = true
	volume.UpdatedAt = hlc.Now()
	volume.UpdatedUserId = r.Context().Value("user").(*model.User).Id
	db.SaveVolume(volume, []string{"Active", "Zone", "NodeId", "UpdatedAt", "UpdatedUserId"})
	service.GetTransport().GossipVolume(volume)
	sse.PublishVolumesChanged(volume.Id)

	rest.WriteResponse(http.StatusOK, w, r, &apiclient.StartVolumeResponse{
		Status: true,
		Zone:   volume.Zone,
	})
}

func HandleVolumeStop(w http.ResponseWriter, r *http.Request) {
	var volume *model.Volume
	var err error

	db := database.GetInstance()

	volumeId := r.PathValue("volume_id")

	// Support lookup by both ID and name
	if validate.UUID(volumeId) {
		// Lookup by ID
		volume, err = db.GetVolume(volumeId)
	} else {
		// Lookup by name
		volume, err = db.GetVolumeByName(volumeId)
	}

	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "volume not found"})
		return
	}

	// Forward to the node where the volume is running (NodeId set for volumes started after cluster support was added)
	if volume.NodeId != "" {
		if shouldForward, nodeId := service.ShouldForwardToNode(volume.NodeId); shouldForward {
			if err := service.ForwardToNode(w, r, nodeId); err != nil {
				rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to forward request"})
			}
			return
		}
	}

	// Use the resolved volume ID for locking
	transport := service.GetTransport()
	unlockToken := transport.LockResource(volume.Id)
	if unlockToken == "" {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to lock volume"})
		return
	}
	defer transport.UnlockResource(volume.Id, unlockToken)

	// Reload to ensure we have the latest state after locking
	volume, err = db.GetVolume(volume.Id)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If the volume is not running or not this server then fail
	cfg := config.GetServerConfig()
	if !volume.Active || volume.Zone != cfg.Zone {
		rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: "volume not running"})
		return
	}

	err = service.GetContainerService().DeleteVolume(volume)
	if err != nil && !strings.Contains(err.Error(), "volume not found") {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	volume.Zone = ""
	volume.Active = false
	volume.UpdatedAt = hlc.Now()
	volume.UpdatedUserId = r.Context().Value("user").(*model.User).Id
	db.SaveVolume(volume, []string{"Active", "Zone", "UpdatedAt", "UpdatedUserId"})
	service.GetTransport().GossipVolume(volume)
	sse.PublishVolumesChanged(volume.Id)

	w.WriteHeader(http.StatusOK)
}
