package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

// generateWebhookSecret returns a 32-byte random hex-encoded HMAC key.
func generateWebhookSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// webhookSecretMasked returns a copy of the given webhook config with its
// secret masked, suitable for list responses.
func webhookSecretMasked(in *model.WebhookConfig) *apiclient.WebhookConfig {
	if in == nil {
		return nil
	}
	return &apiclient.WebhookConfig{
		URL:           in.URL,
		Secret:        service.MaskWebhookSecret(in.Secret),
		Headers:       in.Headers,
		BodyTemplate:  in.BodyTemplate,
		SkipTLSVerify: in.SkipTLSVerify,
	}
}

// webhookSecretUnmasked returns a copy of the given webhook config with its
// secret in plain text, for the single-sink GET (edit form).
func webhookSecretUnmasked(in *model.WebhookConfig) *apiclient.WebhookConfig {
	if in == nil {
		return nil
	}
	return &apiclient.WebhookConfig{
		URL:           in.URL,
		Secret:        in.Secret,
		Headers:       in.Headers,
		BodyTemplate:  in.BodyTemplate,
		SkipTLSVerify: in.SkipTLSVerify,
	}
}

func HandleGetEventSinks(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	// Permission check - return empty list if not authorized (more robust than 403)
	canSeeGlobals := user.HasPermission(model.PermissionManageGlobalEvents)
	canSeeOwn := user.HasPermission(model.PermissionManageEvents)
	if !canSeeGlobals && !canSeeOwn {
		rest.WriteResponse(http.StatusOK, w, r, apiclient.EventSinkList{Count: 0, EventSinks: []apiclient.EventSinkInfo{}})
		return
	}

	db := database.GetInstance()
	sinks, err := db.GetEventSinks()
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	response := apiclient.EventSinkList{
		Count:      0,
		EventSinks: []apiclient.EventSinkInfo{},
	}

	for _, sink := range sinks {
		if sink.IsDeleted {
			continue
		}
		// Visibility: global sinks require the global permission; own sinks
		// require the (own) manage permission and must belong to the user.
		if sink.IsGlobalSink() {
			if !canSeeGlobals {
				continue
			}
		} else {
			if !canSeeOwn || sink.UserId != user.Id {
				continue
			}
		}

		response.EventSinks = append(response.EventSinks, apiclient.EventSinkInfo{
			Id:          sink.Id,
			UserId:      sink.UserId,
			Name:        sink.Name,
			Description: sink.Description,
			Events:      sink.Events,
			SinkType:    sink.SinkType,
			Webhook:     webhookSecretMasked(sink.Webhook),
			ScriptId:    sink.ScriptId,
			Active:      sink.Active,
		})
		response.Count++
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func HandleGetEventSink(w http.ResponseWriter, r *http.Request) {
	sinkId := r.PathValue("event_sink_id")
	if !validate.UUID(sinkId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid event sink ID"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	sink, err := db.GetEventSink(sinkId)
	if err != nil || sink.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Event sink not found"})
		return
	}

	// Permission check
	if sink.IsGlobalSink() {
		if !user.HasPermission(model.PermissionManageGlobalEvents) {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Event sink not found"})
			return
		}
	} else {
		if sink.UserId != user.Id || !user.HasPermission(model.PermissionManageEvents) {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Event sink not found"})
			return
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, apiclient.EventSinkDetails{
		Id:          sink.Id,
		UserId:      sink.UserId,
		Name:        sink.Name,
		Description: sink.Description,
		Events:      sink.Events,
		SinkType:    sink.SinkType,
		Webhook:     webhookSecretUnmasked(sink.Webhook),
		ScriptId:    sink.ScriptId,
		Active:      sink.Active,
	})
}

func HandleCreateEventSink(w http.ResponseWriter, r *http.Request) {
	request := apiclient.EventSinkCreateRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.VarName(request.Name) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid event sink name"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	cfg := config.GetServerConfig()

	// Determine if creating a user-owned sink or a global sink based on request body
	ownerUserId := request.UserId
	if ownerUserId == "current" {
		ownerUserId = user.Id
	}
	isGlobalSink := ownerUserId == ""

	// Permission check (bypass in leaf mode)
	if !cfg.LeafNode {
		if isGlobalSink {
			if !user.HasPermission(model.PermissionManageGlobalEvents) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create global event sinks"})
				return
			}
		} else {
			if ownerUserId != user.Id {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create event sinks for other users"})
				return
			}
			if !user.HasPermission(model.PermissionManageEvents) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create own event sinks"})
				return
			}
		}
	}

	// Normalize the webhook config and auto-generate a secret if one wasn't supplied
	var webhook *model.WebhookConfig
	if request.SinkType == "" || request.SinkType == "webhook" {
		if request.Webhook != nil {
			webhook = &model.WebhookConfig{
				URL:           request.Webhook.URL,
				Secret:        request.Webhook.Secret,
				Headers:       request.Webhook.Headers,
				BodyTemplate:  request.Webhook.BodyTemplate,
				SkipTLSVerify: request.Webhook.SkipTLSVerify,
			}
			if webhook.Secret == "" {
				webhook.Secret = generateWebhookSecret()
			}
		}
	} else if request.SinkType == "json-rpc" {
		if request.Webhook != nil {
			webhook = &model.WebhookConfig{
				BodyTemplate: request.Webhook.BodyTemplate,
			}
		}
	}

	sink := model.NewEventSink(
		request.Name,
		request.Description,
		request.Events,
		request.SinkType,
		webhook,
		request.ScriptId,
		request.Active,
		ownerUserId,
		user.Id,
	)

	db := database.GetInstance()
	err = db.SaveEventSink(sink, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipEventSink(sink)
	service.GetEventDispatcher().ReloadSinks()
	sse.PublishEventSinksChanged(sink.Id)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventEventSinkCreate,
		fmt.Sprintf("Created event sink %s", sink.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"event_sink_id":   sink.Id,
			"event_sink_name": sink.Name,
			"is_global_sink":  isGlobalSink,
		},
	)

	rest.WriteResponse(http.StatusCreated, w, r, &apiclient.EventSinkCreateResponse{
		Status: true,
		Id:     sink.Id,
	})
}

func HandleUpdateEventSink(w http.ResponseWriter, r *http.Request) {
	sinkId := r.PathValue("event_sink_id")
	if !validate.UUID(sinkId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid event sink ID"})
		return
	}

	request := apiclient.EventSinkUpdateRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.VarName(request.Name) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid event sink name"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	cfg := config.GetServerConfig()
	db := database.GetInstance()

	sink, err := db.GetEventSink(sinkId)
	if err != nil || sink.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Event sink not found"})
		return
	}

	// Permission check (bypass in leaf mode)
	if !cfg.LeafNode {
		if sink.IsGlobalSink() {
			if !user.HasPermission(model.PermissionManageGlobalEvents) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit global event sinks"})
				return
			}
		} else {
			if sink.UserId != user.Id {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit this event sink"})
				return
			}
			if !user.HasPermission(model.PermissionManageEvents) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit own event sinks"})
				return
			}
		}
	}

	sink.Name = request.Name
	sink.Description = request.Description
	sink.Events = request.Events
	sink.SinkType = request.SinkType
	sink.ScriptId = request.ScriptId
	sink.Active = request.Active

	// Update the webhook config. If the incoming secret is empty (or the masked
	// value came back unchanged) preserve the existing secret so it isn't wiped.
	if request.SinkType == "" || request.SinkType == "webhook" {
		if request.Webhook != nil {
			existingSecret := ""
			if sink.Webhook != nil {
				existingSecret = sink.Webhook.Secret
			}
			newSecret := request.Webhook.Secret
			if newSecret == "" || newSecret == service.MaskWebhookSecret(existingSecret) {
				newSecret = existingSecret
			}
			sink.Webhook = &model.WebhookConfig{
				URL:           request.Webhook.URL,
				Secret:        newSecret,
				Headers:       request.Webhook.Headers,
				BodyTemplate:  request.Webhook.BodyTemplate,
				SkipTLSVerify: request.Webhook.SkipTLSVerify,
			}
		} else {
			sink.Webhook = nil
		}
	} else if request.SinkType == "json-rpc" {
		if request.Webhook != nil {
			sink.Webhook = &model.WebhookConfig{
				BodyTemplate: request.Webhook.BodyTemplate,
			}
		} else {
			sink.Webhook = nil
		}
	} else {
		sink.Webhook = nil
	}

	sink.UpdatedUserId = user.Id
	sink.UpdatedAt = hlc.Now()

	err = db.SaveEventSink(sink, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipEventSink(sink)
	service.GetEventDispatcher().ReloadSinks()
	sse.PublishEventSinksChanged(sink.Id)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventEventSinkUpdate,
		fmt.Sprintf("Updated event sink %s", sink.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"event_sink_id":   sink.Id,
			"event_sink_name": sink.Name,
			"is_global_sink":  sink.IsGlobalSink(),
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleDeleteEventSink(w http.ResponseWriter, r *http.Request) {
	sinkId := r.PathValue("event_sink_id")
	if !validate.UUID(sinkId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid event sink ID"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	cfg := config.GetServerConfig()
	db := database.GetInstance()

	sink, err := db.GetEventSink(sinkId)
	if err != nil || sink.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Event sink not found"})
		return
	}

	// Permission check (bypass in leaf mode)
	if !cfg.LeafNode {
		if sink.IsGlobalSink() {
			if !user.HasPermission(model.PermissionManageGlobalEvents) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete global event sinks"})
				return
			}
		} else {
			if sink.UserId != user.Id {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete this event sink"})
				return
			}
			if !user.HasPermission(model.PermissionManageEvents) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete own event sinks"})
				return
			}
		}
	}

	sinkName := sink.Name
	sink.Name = sink.Id
	sink.IsDeleted = true
	sink.UpdatedUserId = user.Id
	sink.UpdatedAt = hlc.Now()

	err = db.SaveEventSink(sink, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipEventSink(sink)
	service.GetEventDispatcher().ReloadSinks()
	sse.PublishEventSinksDeleted(sink.Id)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventEventSinkDelete,
		fmt.Sprintf("Deleted event sink %s", sinkName),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"event_sink_id":   sinkId,
			"event_sink_name": sinkName,
			"is_global_sink":  sink.IsGlobalSink(),
		},
	)

	w.WriteHeader(http.StatusOK)
}

// HandleEmitEvent raises a custom event from within a space. It is used by the
// space-side scriptling knot.event.emit() via the apiclient MuxClient.
func HandleEmitEvent(w http.ResponseWriter, r *http.Request) {
	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid space ID"})
		return
	}

	request := apiclient.EmitEventRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if request.Type == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Event type is required"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	space, err := db.GetSpace(spaceId)
	if err != nil || space.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Space not found"})
		return
	}

	if space.UserId != user.Id && !space.IsSharedWith(user.Id) && !user.HasPermission(model.PermissionManageSpaces) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to emit events for this space"})
		return
	}

	service.RaiseCustomEvent("", request.Type, spaceId, space.UserId, request.Payload)

	w.WriteHeader(http.StatusOK)
}

// HandleEmitUserEvent raises a user-scoped custom event from a server-side
// script context (e.g. MCP tool execution) where there is no associated space.
// It is the target of the loopback knot.event.emit() library registered in the
// MCP scriptling environment.
func HandleEmitUserEvent(w http.ResponseWriter, r *http.Request) {
	request := apiclient.EmitEventRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if request.Type == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Event type is required"})
		return
	}

	user := r.Context().Value("user").(*model.User)

	// MCP tool execution has no space context. Use the nil UUID as the space
	// id so downstream consumers (gossip, in-flight records, persistence) see
	// a valid UUID rather than an empty string.
	const nilSpaceID = "00000000-0000-0000-0000-000000000000"
	service.RaiseCustomEvent("", request.Type, nilSpaceID, user.Id, request.Payload)

	w.WriteHeader(http.StatusOK)
}
