package web

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

// helper to save a persistent port forward entry to the space in the database.
func savePortForwardToDB(space *model.Space, entry model.PortForwardEntry) error {
	db := database.GetInstance()

	found := false
	for i := range space.PortForwards {
		if space.PortForwards[i].LocalPort == entry.LocalPort {
			space.PortForwards[i] = entry
			found = true
			break
		}
	}
	if !found {
		space.PortForwards = append(space.PortForwards, entry)
	}

	space.UpdatedAt = hlc.Now()
	if err := db.SaveSpace(space, []string{"PortForwards", "UpdatedAt"}); err != nil {
		return err
	}

	service.GetTransport().GossipSpace(space)
	return nil
}

// helper to remove a persistent port forward entry from the space in the database.
func removePortForwardFromDB(space *model.Space, localPort uint16) error {
	db := database.GetInstance()

	filtered := space.PortForwards[:0]
	for _, pf := range space.PortForwards {
		if pf.LocalPort != localPort {
			filtered = append(filtered, pf)
		}
	}
	space.PortForwards = filtered

	space.UpdatedAt = hlc.Now()
	if err := db.SaveSpace(space, []string{"PortForwards", "UpdatedAt"}); err != nil {
		return err
	}

	service.GetTransport().GossipSpace(space)
	return nil
}

// targetDisplayName renders a resolved forward target for display: the
// space name, or the bare pool name for pool targets.
func targetDisplayName(target *service.ForwardTarget) string {
	if target.Space != nil {
		return target.Space.Name
	}
	if _, name, ok := service.SplitForwardRef(target.Ref); ok {
		return name
	}
	return target.Ref
}

func HandlePortForward(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Read the port forward request
	var request apiclient.PortForwardRequest
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		log.WithError(err).Error("Failed to decode port forward request")
		return
	}

	// Validate the request
	if request.LocalPort < 1 || request.LocalPort > 65535 {
		writeJSONError(w, r, http.StatusBadRequest, "Invalid local port, must be between 1 and 65535")
		return
	}

	if request.RemotePort < 1 || request.RemotePort > 65535 {
		writeJSONError(w, r, http.StatusBadRequest, "Invalid remote port, must be between 1 and 65535")
		return
	}

	if request.Space == "" {
		writeJSONError(w, r, http.StatusBadRequest, "Target space is required")
		return
	}
	if !service.ValidForwardRef(request.Space) {
		writeJSONError(w, r, http.StatusBadRequest, "Invalid target: must be a space or pool name, user--name, or a space ID")
		return
	}

	// Resolve and authorize the target: own space or pool by bare name,
	// another user's space or pool as user--name (ports the target
	// template marks public only), or a space UUID from an older entry.
	target, ferr := service.AuthorizeForwardTarget(user, request.Space, uint16(request.RemotePort))
	if ferr != nil {
		writeJSONError(w, r, ferr.Status, ferr.Message)
		return
	}

	// Get the agent session for the source space
	agentSession := agent_server.GetSession(spaceId)

	// Source space not running — only persistent forwards allowed
	if agentSession == nil && !request.Persistent {
		writeJSONError(w, r, http.StatusConflict, "Space is not running, only persistent forwards can be created for stopped spaces")
		return
	}

	// Validate the target space is running (unless force). Pool targets
	// pick a member per connection, so there is nothing to pre-check.
	if !target.IsPool && !request.Force {
		if agent_server.GetSession(target.Space.Id) == nil {
			writeJSONError(w, r, http.StatusConflict, "Target space is not running")
			return
		}
	}

	// Store the canonical dial reference in DB (survives target space
	// recreation), send the same reference to the agent.
	if agentSession == nil {

		entry := model.PortForwardEntry{
			LocalPort:  request.LocalPort,
			Space:      target.Ref,
			RemotePort: request.RemotePort,
		}

		if err := savePortForwardToDB(space, entry); err != nil {
			log.WithError(err).Error("Failed to save port forward to database")
			writeJSONError(w, r, http.StatusInternalServerError, "Failed to save port forward")
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	// Space is running — forward to agent using the target reference. The
	// agent's own pre-flight checks only understand same-user spaces, so
	// the fact that this request is server-validated rides along as Force.
	portForwardMsg := &msg.PortForwardRequest{
		LocalPort:  uint16(request.LocalPort),
		Space:      target.Ref,
		RemotePort: uint16(request.RemotePort),
		Persistent: request.Persistent,
		Force:      true,
	}

	response, err := agentSession.SendPortForward(portForwardMsg)
	if err != nil {
		log.WithError(err).Error("Failed to send port forward command to agent")
		writeJSONError(w, r, http.StatusInternalServerError, "Failed to send command to agent")
		return
	}

	if response.Success {
		w.WriteHeader(http.StatusOK)
	} else {
		writeJSONError(w, r, http.StatusInternalServerError, response.Error)
	}
}

func HandlePortList(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the agent session
	agentSession := agent_server.GetSession(spaceId)

	if agentSession == nil {
		// Space is not running — return persistent forwards from database
		// Resolve UUIDs to names for display
		forwards := make([]apiclient.PortForwardInfo, 0, len(space.PortForwards))
		for _, pf := range space.PortForwards {
			name := pf.Space
			if targetSpace, err := db.GetSpace(pf.Space); err == nil && targetSpace != nil {
				name = targetSpace.Name
			}
			forwards = append(forwards, apiclient.PortForwardInfo{
				LocalPort:  pf.LocalPort,
				Space:      name,
				RemotePort: pf.RemotePort,
				Persistent: true,
			})
		}

		rest.WriteResponse(http.StatusOK, w, r, apiclient.PortListResponse{
			Forwards: forwards,
		})
		return
	}

	// Space is running — query agent for live forwards
	response, err := agentSession.SendPortList()
	if err != nil {
		log.WithError(err).Error("failed to send port list command to agent")
		writeJSONError(w, r, http.StatusInternalServerError, "Failed to send command to agent")
		return
	}

	forwards := make([]apiclient.PortForwardInfo, 0, len(response.Forwards))
	for _, fwd := range response.Forwards {
		mode := fwd.Mode
		if mode == "" {
			// Older agents default an unused forward to relay; treat an
			// empty mode as unknown rather than relay.
			mode = "—"
		}
		forwards = append(forwards, apiclient.PortForwardInfo{
			LocalPort:   fwd.LocalPort,
			Space:       fwd.Space,
			RemotePort:  fwd.RemotePort,
			Persistent:  fwd.Persistent,
			Mode:        mode,
			LatencyMs:   fwd.LatencyMs,
			JitterMs:    fwd.JitterMs,
			BandwidthKB: fwd.BandwidthKB,
		})
	}

	rest.WriteResponse(http.StatusOK, w, r, apiclient.PortListResponse{
		Forwards: forwards,
	})
}

func HandlePortStop(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Read the port stop request
	var request apiclient.PortStopRequest
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		log.WithError(err).Error("Failed to decode port stop request")
		return
	}

	// Validate the request
	if request.LocalPort < 1 || request.LocalPort > 65535 {
		writeJSONError(w, r, http.StatusBadRequest, "Invalid local port, must be between 1 and 65535")
		return
	}

	// Get the agent session
	agentSession := agent_server.GetSession(spaceId)

	if agentSession == nil {
		// Space is not running — remove from database
		if err := removePortForwardFromDB(space, request.LocalPort); err != nil {
			log.WithError(err).Error("Failed to remove port forward from database")
			writeJSONError(w, r, http.StatusInternalServerError, "Failed to remove port forward")
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	// Space is running — forward to agent (agent handles both live forward + DB removal)
	portStopMsg := &msg.PortStopRequest{
		LocalPort: uint16(request.LocalPort),
	}

	response, err := agentSession.SendPortStop(portStopMsg)
	if err != nil {
		log.WithError(err).Error("Failed to send port stop command to agent")
		writeJSONError(w, r, http.StatusInternalServerError, "Failed to send command to agent")
		return
	}

	if response.Success {
		w.WriteHeader(http.StatusOK)
	} else {
		writeJSONError(w, r, http.StatusInternalServerError, response.Error)
	}
}

func HandlePortThrottle(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	agentSession := agent_server.GetSession(spaceId)
	if agentSession == nil {
		writeJSONError(w, r, http.StatusConflict, "Space is not running")
		return
	}

	var request apiclient.PortThrottleRequest
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		return
	}

	response, err := agentSession.SendThrottle(&msg.ThrottlePortRequest{
		LocalPort:   request.LocalPort,
		LatencyMs:   request.LatencyMs,
		JitterMs:    request.JitterMs,
		BandwidthKB: request.BandwidthKB,
		Reset:       request.Reset,
	})
	if err != nil {
		writeJSONError(w, r, http.StatusInternalServerError, "Failed to send command to agent")
		return
	}

	sse.PublishPortForwardChanged(spaceId, space.UserId)
	rest.WriteResponse(http.StatusOK, w, r, response)
}

func writeJSONError(w http.ResponseWriter, r *http.Request, status int, message string) {
	rest.WriteResponse(status, w, r, map[string]interface{}{
		"success": false,
		"error":   message,
	})
}

func HandlePortApply(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Read the apply request
	var request apiclient.PortApplyRequest
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		log.WithError(err).Error("Failed to decode port apply request")
		return
	}

	if len(request.Forwards) == 0 {
		writeJSONError(w, r, http.StatusBadRequest, "At least one forward is required")
		return
	}

	// Resolve and authorize all targets (bare name, user--name or UUID) and
	// build a lookup
	targetLookup := make(map[string]*service.ForwardTarget)
	for _, fwd := range request.Forwards {
		if fwd.LocalPort < 1 || fwd.LocalPort > 65535 {
			writeJSONError(w, r, http.StatusBadRequest, "Invalid local port, must be between 1 and 65535")
			return
		}
		if fwd.RemotePort < 1 || fwd.RemotePort > 65535 {
			writeJSONError(w, r, http.StatusBadRequest, "Invalid remote port, must be between 1 and 65535")
			return
		}
		if fwd.Space == "" {
			writeJSONError(w, r, http.StatusBadRequest, "Target space is required")
			return
		}
		if !service.ValidForwardRef(fwd.Space) {
			writeJSONError(w, r, http.StatusBadRequest, fmt.Sprintf("Invalid target %q: must be a space or pool name, user--name, or a space ID", fwd.Space))
			return
		}
		if _, seen := targetLookup[fwd.Space]; !seen {
			target, ferr := service.AuthorizeForwardTarget(user, fwd.Space, uint16(fwd.RemotePort))
			if ferr != nil {
				writeJSONError(w, r, ferr.Status, fmt.Sprintf("Target %q: %s", fwd.Space, ferr.Message))
				return
			}
			targetLookup[fwd.Space] = target
		}
	}

	// Get the agent session
	agentSession := agent_server.GetSession(spaceId)

	if agentSession == nil {
		// Space is not running — check that all forwards are persistent, then apply to DB
		for _, fwd := range request.Forwards {
			if !fwd.Persistent {
				writeJSONError(w, r, http.StatusConflict, "Space is not running, only persistent forwards can be created for stopped spaces")
				return
			}
		}

		var applied []apiclient.PortForwardInfo
		var stopped []apiclient.PortForwardInfo

		// Build a set of desired local_ports with canonical target refs
		type resolvedForward struct {
			LocalPort  uint16
			TargetRef  string
			SpaceName  string
			RemotePort uint16
		}
		desiredResolved := make(map[uint16]resolvedForward)
		for _, fwd := range request.Forwards {
			ts := targetLookup[fwd.Space]
			desiredResolved[fwd.LocalPort] = resolvedForward{
				LocalPort:  fwd.LocalPort,
				TargetRef:  ts.Ref,
				SpaceName:  targetDisplayName(ts),
				RemotePort: fwd.RemotePort,
			}
		}

		// Remove forwards not in the desired list or that have changed
		for _, current := range space.PortForwards {
			desired, exists := desiredResolved[current.LocalPort]
			if !exists || current.Space != desired.TargetRef || current.RemotePort != desired.RemotePort {
				currentName := current.Space
				if ts, err := db.GetSpace(current.Space); err == nil && ts != nil {
					currentName = ts.Name
				}
				stopped = append(stopped, apiclient.PortForwardInfo{
					LocalPort:  current.LocalPort,
					Space:      currentName,
					RemotePort: current.RemotePort,
					Persistent: true,
				})
			}
		}

		// Apply all desired forwards to the database (store target refs)
		for _, fwd := range desiredResolved {
			entry := model.PortForwardEntry{
				LocalPort:  fwd.LocalPort,
				Space:      fwd.TargetRef,
				RemotePort: fwd.RemotePort,
			}
			if err := savePortForwardToDB(space, entry); err != nil {
				writeJSONError(w, r, http.StatusInternalServerError, fmt.Sprintf("Failed to save port forward %d: %v", fwd.LocalPort, err))
				return
			}
			applied = append(applied, apiclient.PortForwardInfo{
				LocalPort:  fwd.LocalPort,
				Space:      fwd.SpaceName,
				RemotePort: fwd.RemotePort,
				Persistent: true,
			})
		}

		rest.WriteResponse(http.StatusOK, w, r, apiclient.PortApplyResponse{
			Applied: applied,
			Stopped: stopped,
		})
		return
	}

	// Space is running — use agent for apply
	// Build resolved forwards with target refs for agent communication
	type agentForward struct {
		LocalPort  uint16
		TargetRef  string
		RemotePort uint16
		Persistent bool
		Force      bool
	}
	agentForwards := make(map[uint16]agentForward)
	for _, fwd := range request.Forwards {
		ts := targetLookup[fwd.Space]
		agentForwards[fwd.LocalPort] = agentForward{
			LocalPort:  fwd.LocalPort,
			TargetRef:  ts.Ref,
			RemotePort: fwd.RemotePort,
			Persistent: fwd.Persistent,
			Force:      true, // this request is already server-validated
		}
	}

	currentList, err := agentSession.SendPortList()
	if err != nil {
		log.WithError(err).Error("Failed to get current port list from agent")
		writeJSONError(w, r, http.StatusInternalServerError, "Failed to get current port forwards")
		return
	}

	// Build a map of current forwards keyed by local_port
	currentMap := make(map[uint16]*msg.PortForwardInfo)
	for i := range currentList.Forwards {
		currentMap[currentList.Forwards[i].LocalPort] = &currentList.Forwards[i]
	}

	var stopped []apiclient.PortForwardInfo
	var applied []apiclient.PortForwardInfo
	var errors []string

	// Phase 1: Stop forwards that are not in the desired list or have changed
	for port, current := range currentMap {
		desired, exists := agentForwards[port]
		needsStop := !exists || current.Space != desired.TargetRef || current.RemotePort != desired.RemotePort

		if needsStop {
			stopMsg := &msg.PortStopRequest{LocalPort: port}
			resp, err := agentSession.SendPortStop(stopMsg)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to stop port %d: %v", port, err))
			} else if !resp.Success {
				errors = append(errors, fmt.Sprintf("Failed to stop port %d: %s", port, resp.Error))
			} else {
				stopped = append(stopped, apiclient.PortForwardInfo{
					LocalPort:  current.LocalPort,
					Space:      current.Space,
					RemotePort: current.RemotePort,
					Persistent: current.Persistent,
				})
			}
		}
	}

	// Phase 2: Start forwards that are new or were just stopped
	for _, fwd := range agentForwards {
		current, exists := currentMap[fwd.LocalPort]
		needsStart := !exists || current.Space != fwd.TargetRef || current.RemotePort != fwd.RemotePort

		if needsStart {
			portForwardMsg := &msg.PortForwardRequest{
				LocalPort:  uint16(fwd.LocalPort),
				Space:      fwd.TargetRef,
				RemotePort: uint16(fwd.RemotePort),
				Persistent: fwd.Persistent,
				Force:      fwd.Force,
			}
			resp, err := agentSession.SendPortForward(portForwardMsg)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to forward port %d: %v", fwd.LocalPort, err))
			} else if !resp.Success {
				errors = append(errors, fmt.Sprintf("Failed to forward port %d: %s", fwd.LocalPort, resp.Error))
			} else {
				applied = append(applied, apiclient.PortForwardInfo{
					LocalPort:  uint16(fwd.LocalPort),
					Space:      fwd.TargetRef,
					RemotePort: uint16(fwd.RemotePort),
					Persistent: fwd.Persistent,
				})
			}
		}
	}

	response := apiclient.PortApplyResponse{
		Applied: applied,
		Stopped: stopped,
		Errors:  errors,
	}
	rest.WriteResponse(http.StatusOK, w, r, response)
}
