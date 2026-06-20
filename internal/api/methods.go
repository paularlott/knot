package api

import (
	"encoding/json"
	"errors"
	"io"
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

// HandleCallMethod accepts JSON-RPC 2.0 requests in three forms:
//
//   - Single request: {"jsonrpc":"2.0","method":"...","params":{},"id":1}
//   - Batch:          [{"jsonrpc":"2.0","method":"...","id":1},{...}]
//   - Notification:   {"jsonrpc":"2.0","method":"...","params":{}}  (no id)
//
// Single requests return a single JSON-RPC response object. Batch requests
// return a JSON array of responses (one per request with an id; notifications
// produce no response entry). If all items in a batch are notifications the
// HTTP response is 204 No Content.
//
// Each item in a batch is routed independently through the registry — items
// can target different spaces and the server naturally splits the batch by
// destination agent.
func HandleCallMethod(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		_ = rest.WriteResponse(http.StatusBadRequest, w, r, methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32700, Message: "read error"},
		})
		return
	}

	// Try to decode as a JSON array (batch) first. If that fails, treat as
	// a single request.
	var rawBatch []json.RawMessage
	if json.Unmarshal(body, &rawBatch) == nil && len(rawBatch) > 0 {
		handleBatchCall(w, r, rawBatch, user)
		return
	}
	handleSingleCall(w, r, body, user)
}

// handleSingleCall processes one JSON-RPC request (or notification).
func handleSingleCall(w http.ResponseWriter, r *http.Request, body []byte, user *model.User) {
	var request methods.JSONRPCRequest
	if err := json.Unmarshal(body, &request); err != nil {
		_ = rest.WriteResponse(http.StatusBadRequest, w, r, methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32700, Message: "parse error"},
		})
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

	isNotification := !jsonHasField(body, "id")

	// Route.
	entry, localName, err := methods.DefaultRegistry().Pick(request.Method, user)
	if err != nil {
		writeRouteError(w, r, err, request.ID)
		return
	}
	defer methods.DefaultRegistry().Done(entry)

	session := agent_server.GetSession(entry.SpaceID)
	if session == nil {
		writeNoServerError(w, r, request.ID)
		return
	}

	callReq := &msg.CallMethodRequest{
		Method: localName,
		Params: request.Params,
		ID:     request.ID,
	}

	if isNotification {
		_ = session.SendNotificationMethod(callReq)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	response, err := session.SendCallMethod(callReq, entry.Server.Timeout)
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

// handleBatchCall processes a JSON-RPC batch. Each item is routed
// independently; items can target different spaces. Responses are collected
// preserving original order. Notifications (no id) are forwarded but produce
// no response entry. If all items are notifications, the HTTP response is 204.
func handleBatchCall(w http.ResponseWriter, r *http.Request, rawItems []json.RawMessage, user *model.User) {
	// Pre-allocate a result slot per item so we can preserve ordering.
	// nil entries mean "no response" (notification or error-only).
	results := make([]any, len(rawItems))

	for i, raw := range rawItems {
		var req methods.JSONRPCRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			results[i] = methods.JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   &methods.JSONRPCError{Code: -32700, Message: "parse error"},
			}
			continue
		}
		if req.JSONRPC == "" {
			req.JSONRPC = "2.0"
		}
		if req.Method == "" {
			results[i] = methods.JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   &methods.JSONRPCError{Code: -32600, Message: "method is required"},
			}
			continue
		}

		isNotification := !jsonHasField(raw, "id")

		entry, localName, err := methods.DefaultRegistry().Pick(req.Method, user)
		if err != nil {
			if !isNotification {
				status := "method not found"
				if errors.Is(err, methods.ErrPermission) {
					status = "method not visible to caller"
				}
				results[i] = methods.JSONRPCResponse{
					JSONRPC: "2.0",
					Error:   &methods.JSONRPCError{Code: -32601, Message: status},
					ID:      req.ID,
				}
			}
			continue
		}

		session := agent_server.GetSession(entry.SpaceID)
		if session == nil {
			if !isNotification {
				results[i] = methods.JSONRPCResponse{
					JSONRPC: "2.0",
					Error:   &methods.JSONRPCError{Code: -32000, Message: "no live method server is available"},
					ID:      req.ID,
				}
			}
			methods.DefaultRegistry().Done(entry)
			continue
		}

		callReq := &msg.CallMethodRequest{
			Method: localName,
			Params: req.Params,
			ID:     req.ID,
		}

		if isNotification {
			_ = session.SendNotificationMethod(callReq)
			// No result entry for notifications.
		} else {
			response, err := session.SendCallMethod(callReq, entry.Server.Timeout)
			if err != nil {
				results[i] = methods.JSONRPCResponse{
					JSONRPC: "2.0",
					Error:   &methods.JSONRPCError{Code: -32000, Message: err.Error()},
					ID:      req.ID,
				}
			} else {
				results[i] = response.Response
			}
		}
		methods.DefaultRegistry().Done(entry)
	}

	// Collect non-nil results (requests produce responses; notifications don't).
	responses := []any{}
	for _, r := range results {
		if r != nil {
			responses = append(responses, r)
		}
	}

	if len(responses) == 0 {
		// All items were notifications.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_ = rest.WriteResponse(http.StatusOK, w, r, responses)
}

// jsonHasField reports whether the given raw JSON object contains the named
// top-level key. Used to detect the presence of "id" for JSON-RPC
// notifications (where absent = notification, present = request).
func jsonHasField(raw json.RawMessage, field string) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	_, ok := probe[field]
	return ok
}

func writeRouteError(w http.ResponseWriter, r *http.Request, err error, id any) {
	status := http.StatusNotFound
	message := "method not found"
	if errors.Is(err, methods.ErrPermission) {
		status = http.StatusForbidden
		message = "method not visible to caller"
	}
	log.Debug("POST /api/methods/call: rejected",
		"reason", message,
	)
	_ = rest.WriteResponse(status, w, r, methods.JSONRPCResponse{
		JSONRPC: "2.0",
		Error:   &methods.JSONRPCError{Code: -32601, Message: message},
		ID:      id,
	})
}

func writeNoServerError(w http.ResponseWriter, r *http.Request, id any) {
	_ = rest.WriteResponse(http.StatusServiceUnavailable, w, r, methods.JSONRPCResponse{
		JSONRPC: "2.0",
		Error:   &methods.JSONRPCError{Code: -32000, Message: "no live method server is available"},
		ID:      id,
	})
}
