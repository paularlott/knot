package api

import (
	"context"
	"net/http"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/rest"
)

// HandleGetPluginFieldHandlers lists every declared field handler, for the
// template form's custom field binding popup.
func HandleGetPluginFieldHandlers(w http.ResponseWriter, r *http.Request) {
	registry := plugins.GetRegistry()
	if registry == nil {
		rest.WriteResponse(http.StatusOK, w, r, map[string]any{"handlers": []any{}})
		return
	}
	handlers := registry.FieldHandlers()
	out := make([]map[string]any, 0, len(handlers))
	for _, h := range handlers {
		out = append(out, map[string]any{"id": h.Id, "label": h.Label})
	}
	rest.WriteResponse(http.StatusOK, w, r, map[string]any{"handlers": out})
}

// HandleGetPluginFieldHandlerOptions calls a plugin's declared field
// handler as the requesting user and returns whatever it produced —
// conventionally {"options": [...]} for the bound custom field.
func HandleGetPluginFieldHandlerOptions(w http.ResponseWriter, r *http.Request) {
	registry := plugins.GetRegistry()
	if registry == nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "no plugins loaded"})
		return
	}
	handlerId := r.PathValue("handler_id")
	plugin, handler := registry.FieldHandler(handlerId)
	if plugin == nil || handler == nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "unknown field handler"})
		return
	}

	user := r.Context().Value("user").(*model.User)

	// UseSpaces (the route middleware) is the baseline — field handlers run
	// as part of space forms. A declared permission on the field handler
	// narrows who may invoke it further.
	if handler.Permission != "" && !user.HasPluginPermission(handler.Permission) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "field handler permission not granted"})
		return
	}

	params := map[string]any{"_data": handler.Id}
	if query := r.URL.Query().Get("query"); query != "" {
		params["query"] = query
	}
	timeout := 30 * time.Second
	if cfg := config.GetServerConfig(); cfg != nil && cfg.MCPToolTimeout > 0 {
		timeout = time.Duration(cfg.MCPToolTimeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	result, err := service.DispatchPluginHandler(ctx, apiclient.NewMuxClient(user), user, plugin, handler.Handler, params)
	if err != nil {
		log.Error("field handler", "plugin", plugin.Name, "handler", handler.Handler, "error", err)
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, result)
}
