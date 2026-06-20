package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/methods"
	"github.com/paularlott/knot/internal/util/rest"
)

const (
	// maxBatchSize caps the number of items a single batch request can
	// contain. Larger batches are rejected with -32600 to prevent resource
	// exhaustion on the server and agents.
	maxBatchSize = 100
	// maxConcurrentTargets limits how many different destination agents
	// receive sub-batches concurrently.
	maxConcurrentTargets = 10
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
// return a JSON array of responses. Notifications produce no response entry.
// If all items in a batch are notifications the HTTP response is 204.
func HandleCallMethod(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONRPCError(w, r, -32700, "read error", nil)
		return
	}

	var rawBatch []json.RawMessage
	if json.Unmarshal(body, &rawBatch) == nil && len(rawBatch) > 0 {
		handleBatchCall(w, r, rawBatch, user)
		return
	}
	handleSingleCall(w, r, body, user)
}

// --------------------------------------------------------------------
// Single request / notification
// --------------------------------------------------------------------

func handleSingleCall(w http.ResponseWriter, r *http.Request, body []byte, user *model.User) {
	var request methods.JSONRPCRequest
	if err := json.Unmarshal(body, &request); err != nil {
		writeJSONRPCError(w, r, -32700, "parse error", nil)
		return
	}
	if request.JSONRPC == "" {
		request.JSONRPC = "2.0"
	}
	if request.Method == "" {
		writeJSONRPCError(w, r, -32600, "method is required", request.ID)
		return
	}

	isNotification := !jsonHasField(body, "id")

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
		writeJSONRPCError(w, r, -32000, err.Error(), request.ID)
		return
	}
	_ = rest.WriteResponse(http.StatusOK, w, r, response.Response)
}

// --------------------------------------------------------------------
// Batch
// --------------------------------------------------------------------

// routedItem holds the parse + routing result for one batch item.
type routedItem struct {
	index          int
	request        methods.JSONRPCRequest
	isNotification bool
	entry          *methods.Entry
	localName      string
	session        *agent_server.Session
	errCode        int    // non-zero = item-level error (parse or routing)
	errMsg         string // message for item-level error
}

func handleBatchCall(w http.ResponseWriter, r *http.Request, rawItems []json.RawMessage, user *model.User) {
	if len(rawItems) > maxBatchSize {
		_ = rest.WriteResponse(http.StatusBadRequest, w, r, methods.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &methods.JSONRPCError{Code: -32600, Message: "batch too large"},
		})
		return
	}

	// Phase 1: Parse + route every item sequentially.
	items := make([]routedItem, len(rawItems))
	for i, raw := range rawItems {
		item := routedItem{index: i}

		if err := json.Unmarshal(raw, &item.request); err != nil {
			item.errCode = -32700
			item.errMsg = "parse error"
			items[i] = item
			continue
		}
		if item.request.JSONRPC == "" {
			item.request.JSONRPC = "2.0"
		}
		if item.request.Method == "" {
			item.errCode = -32600
			item.errMsg = "method is required"
			items[i] = item
			continue
		}

		item.isNotification = !jsonHasField(raw, "id")

		entry, localName, err := methods.DefaultRegistry().Pick(item.request.Method, user)
		if err != nil {
			item.errCode = -32601
			item.errMsg = "method not found"
			if errors.Is(err, methods.ErrPermission) {
				item.errMsg = "method not visible to caller"
			}
			items[i] = item
			continue
		}
		item.entry = entry
		item.localName = localName
		item.session = agent_server.GetSession(entry.SpaceID)
		items[i] = item
	}

	// Phase 2: Group forwardable items by destination space.
	type groupInfo struct {
		session  *agent_server.Session
		timeout  int
		entries  []*methods.Entry
		request  msg.CallMethodBatchRequest
		// position maps each sub-batch item index to its original position
		// in the incoming batch, so we can place responses correctly.
		origPositions []int
	}
	groups := map[string]*groupInfo{}
	var groupOrder []string // preserves first-seen order for stable logging

	for i := range items {
		item := &items[i]

		// Item-level errors — handle inline (no forwarding).
		if item.errCode != 0 {
			continue
		}
		if item.session == nil {
			// Agent offline — error for requests, skip notifications.
			if !item.isNotification {
				item.errCode = -32000
				item.errMsg = "no live method server is available"
			}
			if item.entry != nil {
				methods.DefaultRegistry().Done(item.entry)
			}
			continue
		}

		spaceID := item.entry.SpaceID
		g, ok := groups[spaceID]
		if !ok {
			g = &groupInfo{
				session: item.session,
				timeout: item.entry.Server.Timeout,
			}
			groups[spaceID] = g
			groupOrder = append(groupOrder, spaceID)
		}
		g.entries = append(g.entries, item.entry)
		g.request.Items = append(g.request.Items, msg.CallMethodBatchItem{
			Method:         item.localName,
			Params:         item.request.Params,
			ID:             item.request.ID,
			IsNotification: item.isNotification,
		})
		g.origPositions = append(g.origPositions, item.index)
	}

	// Phase 3: Send each group as a sub-batch. Groups run concurrently
	// (up to maxConcurrentTargets at once). Each group uses one yamux
	// stream — items inside are multiplexed by the agent.
	results := make([]any, len(rawItems)) // nil = no response entry
	var wg sync.WaitGroup
	targetSem := make(chan struct{}, maxConcurrentTargets)

	for _, spaceID := range groupOrder {
		g := groups[spaceID]

		// All items are notifications — fire-and-forget per item, no
		// sub-batch needed.
		allNotifications := true
		for _, it := range g.request.Items {
			if !it.IsNotification {
				allNotifications = false
				break
			}
		}

		if allNotifications {
			// Send notifications individually (fire-and-forget, no response
			// expected).
			for _, it := range g.request.Items {
				_ = g.session.SendNotificationMethod(&msg.CallMethodRequest{
					Method:         it.Method,
					Params:         it.Params,
					ID:             it.ID,
					IsNotification: true,
				})
			}
			for _, entry := range g.entries {
				methods.DefaultRegistry().Done(entry)
			}
			continue
		}

		wg.Add(1)
		go func(g *groupInfo) {
			defer wg.Done()
			targetSem <- struct{}{}
			defer func() { <-targetSem }()

			resp, err := g.session.SendCallMethodBatch(&g.request, g.timeout)
			if err != nil {
				// Entire group failed — emit error for each request item.
				respIdx := 0
				for j, it := range g.request.Items {
					if it.IsNotification {
						continue
					}
					origPos := g.origPositions[j]
					results[origPos] = methods.JSONRPCResponse{
						JSONRPC: "2.0",
						Error:   &methods.JSONRPCError{Code: -32000, Message: err.Error()},
						ID:      it.ID,
					}
					respIdx++
				}
			} else {
				// Map responses back to original positions.
				respIdx := 0
				for j, it := range g.request.Items {
					if it.IsNotification {
						continue
					}
					origPos := g.origPositions[j]
					if respIdx < len(resp.Responses) {
						results[origPos] = resp.Responses[respIdx]
					} else {
						results[origPos] = methods.JSONRPCResponse{
							JSONRPC: "2.0",
							Error:   &methods.JSONRPCError{Code: -32000, Message: "missing response from agent"},
							ID:      it.ID,
						}
					}
					respIdx++
				}
			}

			// Release inFlight counters.
			for _, entry := range g.entries {
				methods.DefaultRegistry().Done(entry)
			}
		}(g)
	}
	wg.Wait()

	// Phase 4: Emit item-level errors for requests (notifications get none).
	for i := range items {
		item := &items[i]
		if item.errCode != 0 && !item.isNotification {
			results[item.index] = methods.JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   &methods.JSONRPCError{Code: item.errCode, Message: item.errMsg},
				ID:      item.request.ID,
			}
		}
	}

	// Phase 5: Collect non-nil results, preserving order.
	responses := make([]any, 0, len(rawItems))
	for i := range results {
		if results[i] != nil {
			responses = append(responses, results[i])
		}
	}

	if len(responses) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_ = rest.WriteResponse(http.StatusOK, w, r, responses)
}

// --------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------

func jsonHasField(raw json.RawMessage, field string) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	_, ok := probe[field]
	return ok
}

func writeJSONRPCError(w http.ResponseWriter, r *http.Request, code int, message string, id any) {
	status := http.StatusBadRequest
	if code == -32000 {
		status = http.StatusBadGateway
	} else if code == -32601 {
		status = http.StatusNotFound
	}
	_ = rest.WriteResponse(status, w, r, methods.JSONRPCResponse{
		JSONRPC: "2.0",
		Error:   &methods.JSONRPCError{Code: code, Message: message},
		ID:      id,
	})
}

func writeRouteError(w http.ResponseWriter, r *http.Request, err error, id any) {
	status := http.StatusNotFound
	message := "method not found"
	if errors.Is(err, methods.ErrPermission) {
		status = http.StatusForbidden
		message = "method not visible to caller"
	}
	log.Debug("POST /api/methods/call: rejected", "reason", message)
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
