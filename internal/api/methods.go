package api

import (
	"errors"
	"net/http"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/methods"
	"github.com/paularlott/knot/internal/util/rest"
)

type MethodListResponse struct {
	Count   int                  `json:"count" msgpack:"count"`
	Methods []methods.MethodInfo `json:"methods" msgpack:"methods"`
}

func HandleGetMethods(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	list := methods.DefaultRegistry().List(user)
	log.Debug("GET /api/methods",
		"user", user.Username,
		"user_id", user.Id,
		"visible_count", len(list),
		"registry_total", methods.DefaultRegistry().Count(),
	)
	_ = rest.WriteResponse(http.StatusOK, w, r, MethodListResponse{
		Count:   len(list),
		Methods: list,
	})
}

func HandleCallMethod(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	var request methods.JSONRPCRequest
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		return
	}
	if request.JSONRPC == "" {
		request.JSONRPC = "2.0"
	}
	if request.Method == "" {
		_ = rest.WriteResponse(http.StatusBadRequest, w, r, methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32600, Message: "method is required"},
			ID:      request.ID,
		})
		return
	}

	entry, localName, err := methods.DefaultRegistry().Pick(request.Method, user)
	if err != nil {
		status := http.StatusNotFound
		message := "method not found"
		if errors.Is(err, methods.ErrPermission) {
			status = http.StatusForbidden
			message = "method not visible to caller"
		}
		log.Debug("POST /api/methods/call: rejected",
			"user", user.Username,
			"user_id", user.Id,
			"method", request.Method,
			"reason", message,
		)
		_ = rest.WriteResponse(status, w, r, methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32601, Message: message},
			ID:      request.ID,
		})
		return
	}
	log.Debug("POST /api/methods/call: routed",
		"user", user.Username,
		"method", request.Method,
		"local_name", localName,
		"space_id", entry.SpaceID,
	)
	defer methods.DefaultRegistry().Done(entry)

	session := agent_server.GetSession(entry.SpaceID)
	if session == nil {
		_ = rest.WriteResponse(http.StatusServiceUnavailable, w, r, methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32000, Message: "no live method server is available"},
			ID:      request.ID,
		})
		return
	}

	response, err := session.SendCallMethod(&msg.CallMethodRequest{
		Method: localName,
		Params: request.Params,
		ID:     request.ID,
	}, entry.Server.Timeout)
	if err != nil {
		_ = rest.WriteResponse(http.StatusBadGateway, w, r, methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32000, Message: err.Error()},
			ID:      request.ID,
		})
		return
	}

	_ = rest.WriteResponse(http.StatusOK, w, r, response.Response)
}
